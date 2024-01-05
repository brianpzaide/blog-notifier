package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	_ "github.com/mattn/go-sqlite3"
)

const (
	BLOGS_DB           = "./blogs.sqlite3"
	MAIL_MESSAGE       = `New blog post %s on blog %s`
	CREATE_BLOGS_TABLE = `CREATE TABLE IF NOT EXISTS blogs (
		site                    VARCHAR(256) PRIMARY KEY,
		last_link               VARCHAR(256)
	)`
	CREATE_POSTS_TABLE = `CREATE TABLE IF NOT EXISTS posts (
		site    VARCHAR(256),
		link    VARCHAR(256),
		FOREIGN KEY (site) REFERENCES blogs(site) ON DELETE CASCADE
	)`
	CREATE_MAILS_TABLE = `CREATE TABLE IF NOT EXISTS mails (
		id      INTEGER PRIMARY KEY AUTOINCREMENT,
		mail    TEXT,
		is_sent INTEGER DEFAULT 0
	)`
	REMOVE_SITE          = `DELETE from blogs WHERE site = ?`
	ADD_NEW_BLOG         = `INSERT INTO blogs (site, last_link) VALUES(?, ?)`
	UPDATE_BLOG          = `UPDATE blogs SET last_link = ? WHERE site = ?`
	UPDATE_MAIL          = `UPDATE mails SET is_sent = 1 WHERE id = ?`
	ADD_NEW_POST         = `INSERT INTO posts (site, link) VALUES(?, ?)`
	ADD_NEW_MAIL         = `INSERT INTO mails (mail) VALUES(?)`
	FETCH_BLOGS          = `SELECT * FROM blogs`
	FETCH_POSTS          = `SELECT * FROM posts`
	FETCH_MAILS          = `SELECT id, mail FROM mails WHERE is_sent = 0`
	IS_BLOG              = `SELECT 1 FROM blogs WHERE site = ?`
	IS_POST              = `SELECT 1 FROM posts WHERE site = ? and link = ?`
	FETCH_POSTS_FOR_BLOG = `SELECT link FROM posts WHERE site = ?`
)

type emailServer struct {
	Host string
	Port int
}

type emailClient struct {
	Email    string
	Password string
	SendTo   string `yaml:"send_to"`
}

type blogNotifierConfig struct {
	Server emailServer
	Client emailClient
}

type blogPostsLink struct {
	site string
	link string
}

type mailStruct struct {
	id  int
	msg string
}

// Getting database connection, It is important to note that To enable foreign key support in SQLite,
// you need to ensure that the foreign key constraints are enabled for each database connection.
// This must be done after opening a database connection using SQLite.
func getDBConnection() (*sql.DB, error) {
	// os.Remove(BLOGS_DB)
	db, err := sql.Open("sqlite3", BLOGS_DB)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		fmt.Println("Error enabling foreign key constraints:", err)
		return nil, err
	}
	return db, nil
}

// Creating Database tables //
func migrate() error {

	db, err := getDBConnection()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(CREATE_BLOGS_TABLE)
	if err != nil {
		fmt.Println("error creating blogs table")
		return err
	}

	_, err = db.Exec(CREATE_POSTS_TABLE)
	if err != nil {
		fmt.Println("error creating posts table")
		return err
	}

	_, err = db.Exec(CREATE_MAILS_TABLE)
	if err != nil {
		fmt.Println("error creating mails table")
		return err
	}
	return nil
}

func entityExists(query string, args ...any) (bool, error) {
	// does the blog with name 'site' exists
	db, err := getDBConnection()
	if err != nil {
		return false, err
	}
	defer db.Close()
	row := db.QueryRow(query, args...)
	i := -1
	err = row.Scan(&i)

	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return i >= 0, nil
}

func blogExists(site string) (bool, error) {
	return entityExists(IS_BLOG, site)
}

func postExists(site, post string) (bool, error) {
	return entityExists(IS_POST, site, post)
}

// function to add a new site
func addNewSite(site, link string) error {
	db, err := getDBConnection()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(ADD_NEW_BLOG, site, link)
	if err != nil {
		return err
	}
	return nil
}

// first check if the post already exists in the database, if not insert a new entry
func addNewPostIfNotExist(site, link string) (bool, error) {
	db, err := getDBConnection()
	if err != nil {
		return false, err
	}
	defer db.Close()
	row := db.QueryRow(IS_POST, site, link)
	i := -1
	err = row.Scan(&i)

	if err != nil {
		if err == sql.ErrNoRows {
			_, err = db.Exec(ADD_NEW_POST, site, link)
			if err == nil {
				return true, err
			}
		}
		return false, err
	}

	return false, nil
}

// functionality to add new mails, new mails containg info about new posts that users need to be notified about
func addMail(site, link string) error {
	db, err := getDBConnection()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(ADD_NEW_MAIL, fmt.Sprintf(MAIL_MESSAGE, link, site))
	if err != nil {
		return err
	}
	return nil
}

// list all the the sites the user is subscribing to
func listAllSites() (map[string]string, error) {
	// list all the sites that are saved to the database
	db, err := getDBConnection()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(FETCH_BLOGS)
	if err != nil {
		return nil, err
	}
	blogAndLastLink := make(map[string]string, 0)
	for rows.Next() {
		_site, last_link := "", ""
		rows.Scan(&_site, &last_link)

		// postLinks = append(postLinks, _site)
		blogAndLastLink[_site] = last_link
		fmt.Printf("retrieved site: %s, last_link: %s\n", _site, last_link)
	}
	return blogAndLastLink, nil
}

func getPostsForSite(site string) ([]string, error) {
	db, err := getDBConnection()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(FETCH_POSTS_FOR_BLOG, site)
	if err != nil {
		return nil, err
	}
	existingPosts := make([]string, 0)
	for rows.Next() {
		_l := ""
		err := rows.Scan(&_l)
		if err == nil {
			existingPosts = append(existingPosts, _l)
		}
	}
	return existingPosts, nil
}

// implements a functionality to remove a site
func removeSite(site string) error {
	// remove a site from the watch list
	db, err := getDBConnection()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(REMOVE_SITE, site)
	if err != nil {
		fmt.Printf("error deleting a site %s from the blogs table\n", site)
		return err
	}
	return nil
}

// updates the last visited site if new post in the blog site
func updateLastSiteVisited(site, link string) error {
	// remove a site from the watch list
	db, err := getDBConnection()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(UPDATE_BLOG, link, site)
	if err != nil {
		fmt.Printf("error updating last_link %s for blog %s in the blogs table\n", link, site)
		return err
	}
	return nil
}

// finds all the links in a blog post
func findAllLinks(site string) ([]string, error) {
	res, err := http.Get(site)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, err
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	links := make([]string, 0)

	// Find the review items
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		// For each item found, get the title
		link, exists := s.Attr("href")
		if exists {
			links = append(links, link)
		}
	})
	return links, nil
}

// recursive crawl function
func _crawl(site, link string, links *[]blogPostsLink) error {
	_links, err := findAllLinks(link)
	if err == nil {
		for _, _link := range _links {
			*links = append(*links, blogPostsLink{
				site: site,
				link: _link,
			})
			err := _crawl(site, _link, links)
			if err != nil {
				return fmt.Errorf("%s: error in recursive crawl", site)
			}
		}
		return nil
	} else {
		return fmt.Errorf("%s: error in finAllLinks", site)
	}
}

// implements the crawl functionality
func crawl() (map[string][]string, error) {
	// crawl the sites

	// get the all the blogs
	blogs, err := listAllSites()
	if err != nil {
		fmt.Printf("error fetching items from blogs table\n")
		return nil, err
	}

	postsCh := make(chan []blogPostsLink)
	errCh := make(chan error)
	wg := &sync.WaitGroup{}

	for _, blog := range blogs {
		wg.Add(1)
		go func(site string) {
			defer wg.Done()
			links := make([]blogPostsLink, 0)
			err := _crawl(site, site, &links)
			if err != nil {
				errCh <- err
			} else {
				postsCh <- links
			}
			if n := len(links) - 1; n > 0 {
				err = updateLastSiteVisited(site, links[n].link)
				if err != nil {
					errCh <- err
				}
			}

		}(blog)
	}

	go func() {
		wg.Wait()
		close(postsCh)
		close(errCh)
	}()

	siteLinksMap := make(map[string][]string)

	for linksSlice := range postsCh {
		blog := linksSlice[0].site
		_, ok := siteLinksMap[blog]
		if !ok {
			siteLinksMap[blog] = make([]string, 0)
		}
		for _, link := range linksSlice {
			siteLinksMap[blog] = append(siteLinksMap[blog], link.link)
		}
	}
	for err := range errCh {
		fmt.Println(err)
	}
	return siteLinksMap, nil
}

func run() error {
	// crawl
	site_links_map, err := crawl()
	if err != nil {
		return err
	}
	// update the database for the new posts
	for blog, posts := range site_links_map {
		for _, post := range posts {
			ok, err := addNewPostIfNotExist(blog, post)
			if err != nil {
				return err
			}
			if ok {
				err = addMail(blog, post)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func main() {
	// Parse command-line arguments
	migrateFlag := flag.Bool("migrate", false, "Create sqlite3 database and prepare tables")
	exploreFlag := flag.String("explore", "", "Add site to watchlist")
	listFlag := flag.Bool("list", false, "List saved sites")
	removeFlag := flag.String("remove", "", "Remove site from watchlist")
	crawlFlag := flag.Bool("crawl", false, "Crawl all the blog sites curently in the blogs table (watchlist)")

	listPostsFlag := flag.NewFlagSet("listPosts", flag.ExitOnError)
	updateFlag := flag.NewFlagSet("updateLastLink", flag.ExitOnError)

	// Define multiple flags for the FlagSet
	var (
		flagBlogSite = updateFlag.String("site", "", "web address of the blog site")
		flagLastLink = updateFlag.String("post", "", "web address of the latest blog post")
		flagSite     = listPostsFlag.String("site", "", "web address of the blog site")
	)

	fmt.Println(strings.Join(os.Args, " "))

	// Check if command and flags are provided
	if len(os.Args) <= 3 {
		flag.Parse()

		if *migrateFlag {
			fmt.Println("migrate")
			err := migrate()
			if err != nil {
				log.Fatal(err)
			}
		}

		if *exploreFlag != "" {
			fmt.Println("explore")
			if err := addNewSite(*exploreFlag, *exploreFlag); err != nil {
				log.Fatal(err)
			}
		}

		if *listFlag {
			fmt.Println("list")
			sites, err := listAllSites()
			if err != nil {
				log.Fatal(err)
			}
			for site, lastLink := range sites {
				fmt.Printf("%s %s\n", site, lastLink)
			}
		}
		if *removeFlag != "" {
			fmt.Println("remove")
			if err := removeSite(*removeFlag); err != nil {
				log.Fatal(err)
			}
		}
		if *crawlFlag {
			fmt.Println("crawl")
			if err := run(); err != nil {
				log.Fatal(err)
			}
		}
	} else if os.Args[1] == "updateLastLink" {
		updateFlag.Parse(os.Args[2:])

		// Check individual flags
		if *flagBlogSite != "" && *flagLastLink != "" {
			err := updateLastSiteVisited(*flagBlogSite, *flagLastLink)
			if err != nil {
				log.Fatal(err)
			}
		}
	} else if os.Args[1] == "listPosts" {
		listPostsFlag.Parse(os.Args[2:])
		if *flagSite != "" {
			blogPosts, err := getPostsForSite(*flagSite)
			if err != nil {
				log.Fatal(err)
			}
			for _, bp := range blogPosts {
				fmt.Println(bp)
			}
		}
	} else {
		fmt.Println("Invalid command")
		os.Exit(1)
	}
}

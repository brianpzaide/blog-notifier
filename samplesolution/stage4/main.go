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
	MAX_DEPTH          = 3
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

type blogPostsLink struct {
	site string
	link string
}

// Getting database connection, It is important to note that To enable foreign key support in SQLite,
// you need to ensure that the foreign key constraints are enabled for each database connection.
// This must be done after opening a database connection using SQLite.
func getDBConnection() (*sql.DB, error) {
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
		return fmt.Errorf("error creating blogs table")
	}

	_, err = db.Exec(CREATE_POSTS_TABLE)
	if err != nil {
		return fmt.Errorf("error creating posts table")
	}

	_, err = db.Exec(CREATE_MAILS_TABLE)
	if err != nil {
		return fmt.Errorf("error creating mails table")
	}
	return nil
}

// does the blog with name 'site' exists
func entityExists(query string, args ...any) (bool, error) {
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
	ok, err := blogExists(site)
	if err != nil {
		return err
	}
	if !ok {
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
	return fmt.Errorf("%s already exists in the watch list", site)
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
		fmt.Println(err.Error())
		if err == sql.ErrNoRows {
			fmt.Println("the error is sql.ErrNoRows")
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
		blogAndLastLink[_site] = last_link
	}
	return blogAndLastLink, nil
}

// list all the the blog posts belonging to the blog site that is in the watchlist
func getPostsForSite(site string) ([]string, error) {
	ok, err := blogExists(site)
	if err != nil {
		return nil, err
	}
	if ok {
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
	return nil, fmt.Errorf("%s does not exist in the watch list", site)

}

// implements a functionality to remove a site from the watchlist
func removeSite(site string) error {
	ok, err := blogExists(site)
	if err != nil {
		return err
	}
	if ok {
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
	return fmt.Errorf("%s does not exist in the watch list", site)
}

// updates the last visited site if new post is added to the blog
func updateLastSiteVisited(site, link string) error {
	ok, err := blogExists(site)
	if err != nil {
		return err
	}
	if ok {
		db, err := getDBConnection()
		if err != nil {
			return err
		}
		defer db.Close()
		_, err = db.Exec(UPDATE_BLOG, link, site)
		if err != nil {
			if err == sql.ErrNoRows {
				fmt.Printf("%s does not exist in the watchlist", site)
				return nil
			}
			fmt.Printf("error updating last_link %s for blog %s in the blogs table\n", link, site)
			return err
		}
		return nil
	}
	return fmt.Errorf("%s does not exist in the watch list", site)
}

// finds all the links in a blog post
func findAllLinks(site string) ([]string, error) {
	res, err := http.Get(site)
	if err != nil {
		return nil, fmt.Errorf("could not reach the site: %s", site)
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

	// Initialize an empty slice to store discovered links
	links := make([]string, 0)

	// Iterate over all 'a' (anchor) elements in the HTML document
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		// Extract the 'href' attribute value from each 'a' element
		link, exists := s.Attr("href")
		if exists {
			// Add the discovered link to the slice
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

// implements the crawl functionality. Crawls all the sites in the watch list
func crawl() (map[string][]string, error) {
	// get the all the blogs
	blogs, err := listAllSites()
	if err != nil {
		return nil, fmt.Errorf("error fetching items from blogs table\n")
	}

	postsCh := make(chan []blogPostsLink, len(blogs))
	errCh := make(chan error, len(blogs))
	doneCh := make(chan bool, 0)
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
		close(doneCh)
	}()

	siteLinksMap := make(map[string][]string)

	for {
		select {
		case linksSlice, is_open := <-postsCh:
			if !is_open {
				postsCh = nil
			}
			if len(linksSlice) == 0 {
				continue
			}
			blog := linksSlice[0].site
			_, ok := siteLinksMap[blog]
			if !ok {
				siteLinksMap[blog] = make([]string, 0)
			}
			for _, link := range linksSlice {
				siteLinksMap[blog] = append(siteLinksMap[blog], link.link)
			}
		case err := <-errCh:
			fmt.Println(err)
		case <-doneCh:
			return siteLinksMap, nil
		}
	}

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

	listPostsFlag := flag.NewFlagSet("list-posts", flag.ExitOnError)
	updateFlag := flag.NewFlagSet("update-last-link", flag.ExitOnError)

	// Define multiple flags for the FlagSet
	var (
		flagBlogSite = updateFlag.String("site", "", "web address of the blog site")
		flagLastLink = updateFlag.String("post", "", "web address of the latest blog post")
		flagSite     = listPostsFlag.String("site", "", "web address of the blog site")
	)

	fmt.Println(strings.Join(os.Args, " "))

	// Check if command and flags are provided
	if len(os.Args) < 2 {
		log.Println("no command input specified")
		return
	}

	if len(os.Args) <= 3 {
		flag.Parse()

		if *migrateFlag {
			err := migrate()
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			fmt.Println("Database 'blogs.sqlite3' created successfully")
			fmt.Println("Tables 'blogs', 'posts', and 'mails' initialized")
			return
		}

		if *exploreFlag != "" {
			if err := addNewSite(*exploreFlag, *exploreFlag); err != nil {
				fmt.Println(err.Error())
				return
			}
			fmt.Println("New blog added to watchlist:")
			fmt.Printf("site: %s\n", *exploreFlag)
			fmt.Printf("last link: %s\n", *exploreFlag)
			return
		}

		if *listFlag {
			sites, err := listAllSites()
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			for site, lastPost := range sites {
				fmt.Printf("%s %s\n", site, lastPost)
			}
		}
		if *removeFlag != "" {
			if err := removeSite(*removeFlag); err != nil {
				fmt.Println(err.Error())
				return
			}
			fmt.Printf("%s removed from the watch list.\n", *removeFlag)
			return
		}
		if *crawlFlag {
			if err := run(); err != nil {
				log.Fatal(err)
			}
		}
	} else if os.Args[1] == "update-last-link" {
		updateFlag.Parse(os.Args[2:])

		// Check individual flags
		if *flagBlogSite != "" && *flagLastLink != "" {
			err := updateLastSiteVisited(*flagBlogSite, *flagLastLink)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			fmt.Printf("The last link for %s updated to %s\n", *flagBlogSite, *flagLastLink)
		} else {
			fmt.Println("For 'update-last-link' sub-command, flags 'site' and 'post' cannot be empty")
		}
	} else if os.Args[1] == "list-posts" {
		listPostsFlag.Parse(os.Args[2:])
		if *flagSite != "" {
			blogPosts, err := getPostsForSite(*flagSite)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			for _, bp := range blogPosts {
				fmt.Println(bp)
			}
		} else {
			fmt.Println("The flag site cannot be empty")
		}
	} else {
		fmt.Println("Invalid command")
		os.Exit(1)
	}
}

package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/PuerkitoBio/goquery"
	_ "github.com/mattn/go-sqlite3"
)

const (
	MAX_DEPTH          = 3
	CONFIG_FILE        = "./credentials.yml"
	BLOGS_DB           = "./blogs.sqlite3"
	MAIL_MESSAGE       = `New blog post %s on blog %s`
	CREATE_BLOGS_TABLE = `CREATE TABLE IF NOT EXISTS blogs (
		site                    TEXT PRIMARY KEY,
		last_link               TEXT
	)`
	CREATE_POSTS_TABLE = `CREATE TABLE IF NOT EXISTS posts (
		site    TEXT,
		link    TEXT,
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

type telegramConfig struct {
	Channel  string
	BotToken string `yaml:"bot_token"`
}

type blogNotifierConfig struct {
	Mode     string
	Server   emailServer
	Client   emailClient
	Telegram telegramConfig
}

type blogPostsLink struct {
	site string
	link string
}

type mailStruct struct {
	id  int
	msg string
}

var conf blogNotifierConfig
var (
	mailAddr, sender, recipient, password string
)

// Parsing Config File  //
func parseConfig(configFile string) error {
	b, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("file '%s' not found", configFile)
	}

	conf = blogNotifierConfig{}

	err = yaml.Unmarshal(b, &conf)
	if err != nil {
		return fmt.Errorf("error unmarshalling the config file %s", configFile)
	}

	mailAddr = fmt.Sprintf("%s:%d", conf.Server.Host, conf.Server.Port)
	sender, password, recipient = conf.Client.Email, conf.Client.Password, conf.Client.SendTo
	fmt.Printf("mode: %s\n", conf.Mode)
	fmt.Printf("email_server: %s\n", mailAddr)
	fmt.Printf("client: %s %s %s\n", sender, password, recipient)
	fmt.Printf("telegram: %s@%s\n", conf.Telegram.BotToken, conf.Telegram.Channel)
	return nil
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

// fetches all the mails that are not yet sent to the user
func fetchMails() ([]mailStruct, error) {
	db, err := getDBConnection()
	defer db.Close()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(FETCH_MAILS)
	if err != nil {
		return nil, err
	}
	mails := make([]mailStruct, 0)
	for rows.Next() {
		_id, _mail := 0, ""
		err := rows.Scan(&_id, &_mail)
		if err == nil {
			mails = append(mails, mailStruct{
				id:  _id,
				msg: _mail,
			})
		} else {
			return nil, err
		}
	}
	return mails, nil
}

// fetches posts that are already existing in the database
func getExistingPosts() (map[string][]string, error) {
	db, err := getDBConnection()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(FETCH_POSTS)
	if err != nil {
		return nil, err
	}
	existingPosts := make(map[string][]string)
	for rows.Next() {
		_s, _l := "", ""
		err := rows.Scan(&_s, &_l)
		if err == nil {
			_, ok := existingPosts[_s]
			if !ok {
				existingPosts[_s] = make([]string, 0)
			}
			existingPosts[_s] = append(existingPosts[_s], _l)
		}
	}
	return existingPosts, nil
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
			return err
		}
		return nil
	}
	return fmt.Errorf("%s does not exist in the watch list", site)
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

// after sending the mails mark the mails in the database as sent
func updateMail(id int) error {
	// remove a site from the watch list
	db, err := getDBConnection()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(UPDATE_MAIL, id)
	if err != nil {
		fmt.Printf("error updating is_sent id %d in the mails table\n", id)
		return err
	}
	return nil
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

// fetches mails that need to be sent, sends the mails, updates the database if the mail is sent successfully
func notify() error {
	// fetching all the new messages or messages that are not sent
	mails, err := fetchMails()
	if err != nil {
		return err
	}
	deliveredCh := make(chan int)
	errCh := make(chan error)
	wg := &sync.WaitGroup{}
	// send email notification to the user
	for _, mail := range mails {
		wg.Add(1)
		go func(_mail mailStruct) {
			defer wg.Done()
			err := smtp.SendMail(mailAddr, nil, sender, []string{recipient}, []byte(_mail.msg))
			if err == nil {
				deliveredCh <- _mail.id
			} else {
				errCh <- err
			}
		}(mail)
	}

	go func() {
		wg.Wait()
		close(deliveredCh)
		close(errCh)
	}()

	for id := range deliveredCh {
		err = updateMail(id)
	}
	for err := range errCh {
		fmt.Println("error delivering mail")
		fmt.Println(err)
	}

	return nil
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
			} else {
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
			}

		case err, is_open := <-errCh:
			if !is_open {
				errCh = nil
			} else {
				fmt.Println(err)
			}

		case <-doneCh:
			return siteLinksMap, nil
		}
	}
}

func syncBlogs(configFile string) error {
	err := parseConfig(configFile)
	if err != nil {
		return err
	}
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
	// notify the user about the new sites
	err = notify()
	if err != nil {
		return err
	}
	return nil
}

func main() {
	// Parse command-line arguments
	migrateFlag := flag.Bool("migrate", false, "Create sqlite3 database and prepare tables")
	exploreFlag := flag.String("explore", "", "Add site to watchlist")
	listFlag := flag.Bool("list", false, "List saved sites")
	removeFlag := flag.String("remove", "", "Remove site from watchlist")

	listPostsCommand := flag.NewFlagSet("list-posts", flag.ExitOnError)
	syncCommand := flag.NewFlagSet("sync", flag.ExitOnError)

	// Define multiple flags for the FlagSet
	var (
		flagConfig = syncCommand.String("conf", "", "config file name")
		flagSite   = listPostsCommand.String("site", "", "web address of the blog site")
	)

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
	} else if os.Args[1] == "sync" {
		syncCommand.Parse(os.Args[2:])
		if *flagConfig != "" {
			err := syncBlogs(*flagConfig)
			if err != nil {
				fmt.Println(err.Error())
			}
		} else {
			fmt.Println("The flag conf cannot be empty")
			return
		}
	} else if os.Args[1] == "list-posts" {
		listPostsCommand.Parse(os.Args[2:])
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

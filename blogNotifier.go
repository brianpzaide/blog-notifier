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

	"github.com/PuerkitoBio/goquery"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/yaml.v3"
)

const (
	CONFIG_FILE        = "./credentials.yml"
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
	host string
	port int
}

type emailClient struct {
	sender   string
	password string
	receiver string
}

type blogNotifierConfig struct {
	eServer emailServer
	eClient emailClient
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

func parseConfig() error {
	f, err := os.Open(CONFIG_FILE)
	if err != nil {
		fmt.Printf("error opening config file %s\n", CONFIG_FILE)
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			f.Close()
		}
	}()
	b := make([]byte, 1024)
	n, err := f.Read(b)
	if err != nil {
		fmt.Printf("error reading the config file %s\n", CONFIG_FILE)
		return err
	}

	err = yaml.Unmarshal(b[0:n], &conf)
	if err != nil {
		fmt.Printf("error unmarshalling the config file %s", CONFIG_FILE)
		return err
	}
	return nil
}

func getDBConnection() *sql.DB {
	// os.Remove(BLOGS_DB)
	db, err := sql.Open("sqlite3", BLOGS_DB)
	if err != nil {
		log.Fatal(err)
	}
	return db
}

func migrate() {
	// create database tables
	db := getDBConnection()
	defer func() {
		if r := recover(); r != nil {
			db.Close()
		}
	}()
	_, err := db.Exec(CREATE_BLOGS_TABLE)
	if err != nil {
		fmt.Println("error creating blogs table")
		panic(err)
	}

	_, err = db.Exec(CREATE_POSTS_TABLE)
	if err != nil {
		fmt.Println("error creating posts table")
		panic(err)
	}

	_, err = db.Exec(CREATE_MAILS_TABLE)
	if err != nil {
		fmt.Println("error creating mails table")
		panic(err)
	}
}

func blog_exists(site string) (bool, error) {
	// does the blog with name 'site' exists
	db := getDBConnection()
	defer db.Close()
	row := db.QueryRow(IS_BLOG, site)
	i := -1
	err := row.Scan(&i)

	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return i >= 0, nil

}

func post_exists(site, post string) (bool, error) {
	// does the blog with name 'site' exists
	db := getDBConnection()
	defer db.Close()
	row := db.QueryRow(IS_POST, site, post)
	i := -1
	err := row.Scan(&i)

	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return i >= 0, nil

}

func add_new_site(site, link string) error {
	db := getDBConnection()
	defer db.Close()
	_, err := db.Exec(ADD_NEW_BLOG, site, link)
	if err != nil {
		return err
	}
	return nil
}

func add_new_post_if_not_exist(site, link string) (bool, error) {
	db := getDBConnection()
	defer db.Close()
	row := db.QueryRow(IS_POST, site, link)
	i := -1
	err := row.Scan(&i)

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

func add_mail(site, link string) error {
	db := getDBConnection()
	defer db.Close()
	_, err := db.Exec(ADD_NEW_MAIL, fmt.Sprintf(MAIL_MESSAGE, site, link))
	if err != nil {
		return err
	}
	return nil
}

func list_all_sites() ([]string, error) {
	// list all the sites that are saved to the database
	postLinks := make([]string, 0)
	db := getDBConnection()
	defer db.Close()
	rows, err := db.Query(FETCH_BLOGS)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		_site, last_link := "", ""
		rows.Scan(&_site, &last_link)
		postLinks = append(postLinks, _site)
		fmt.Printf("retrieved site: %s, last_link: %s\n", _site, last_link)
	}
	return postLinks, nil
}

func fetch_mails() ([]mailStruct, error) {
	db := getDBConnection()
	defer db.Close()
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

func getExistingPosts() (map[string][]string, error) {
	db := getDBConnection()
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

func remove_site(site string) error {
	// remove a site from the watch list
	db := getDBConnection()
	defer db.Close()
	_, err := db.Exec(REMOVE_SITE, site)
	if err != nil {
		fmt.Printf("error deleting a site %s from the blogs table\n", site)
		return err
	}
	return nil
}

func update_last_site_visited(site, link string) error {
	// remove a site from the watch list
	db := getDBConnection()
	defer db.Close()
	_, err := db.Exec(UPDATE_BLOG, link, site)
	if err != nil {
		fmt.Printf("error updating last_link %s for blog %s in the blogs table\n", link, site)
		return err
	}
	return nil
}

func update_mail(id int) error {
	// remove a site from the watch list
	db := getDBConnection()
	defer db.Close()
	_, err := db.Exec(UPDATE_MAIL, id)
	if err != nil {
		fmt.Printf("error updating is_sent id %d in the mails table\n", id)
		return err
	}
	return nil
}

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

func explore(newSite string) {
	// first check if the site exists
	ok, err := blog_exists(newSite)
	if err != nil {
		panic(fmt.Sprintf("error checking whether a site %s exists in the blogs table\n", newSite))
	}
	// if the blog does not exist in the blogs table, then insert a new item in the blogs table else noop
	if !ok {
		links, err := findAllLinks(newSite)
		if err != nil {
			fmt.Printf("error finding links in a new site %s \n", newSite)
			panic(err)
		}
		last_link := ""
		if len(links) > 0 {
			last_link = links[0]
		}
		err = add_new_site(newSite, last_link)
		if err != nil {
			fmt.Printf("error adding a new site %s into the blogs table\n", newSite)
			panic(err)
		}
	}
}

func notify() error {
	// fetching all the new messages or messages that are not sent
	mails, err := fetch_mails()
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
			mail_addr := fmt.Sprintf("%s:%d", conf.eServer.host, conf.eServer.port)
			err := smtp.SendMail(mail_addr, nil, conf.eClient.sender, []string{conf.eClient.receiver}, []byte(_mail.msg))
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
		err = update_mail(id)
	}
	for err := range errCh {
		fmt.Println("error delivering mail")
		fmt.Println(err)
	}

	return nil
}

func _crawl(site, link string, links []blogPostsLink) error {
	_links, err := findAllLinks(link)
	if err == nil {
		for _, _link := range _links {
			links = append(links, blogPostsLink{
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

func crawl() map[string][]string {
	// crawl the sites

	// get the all the blogs
	blogs, err := list_all_sites()
	if err != nil {
		fmt.Printf("error fetching items from blogs table")
		panic(err)
	}

	postsCh := make(chan []blogPostsLink)
	errCh := make(chan error)
	wg := &sync.WaitGroup{}

	for _, blog := range blogs {
		wg.Add(1)
		go func(site string) {
			defer wg.Done()
			links := make([]blogPostsLink, 0)
			err := _crawl(site, site, links)
			if err != nil {
				errCh <- err
			} else {
				postsCh <- links
			}
		}(blog)
	}

	go func() {
		wg.Wait()
		close(postsCh)
		close(errCh)
	}()

	site_links_map := make(map[string][]string)

	for linksSlice := range postsCh {
		blog := linksSlice[0].site
		_, ok := site_links_map[blog]
		if !ok {
			site_links_map[blog] = make([]string, 0)
		}
		for _, link := range linksSlice {
			site_links_map[blog] = append(site_links_map[blog], link.link)
		}
	}
	for err := range errCh {
		fmt.Println(err)
	}

	return site_links_map

}

func run() {
	// parse the config
	parseConfig()
	// crawl
	site_links_map := crawl()
	// update the database for the new posts
	for blog, posts := range site_links_map {
		for _, post := range posts {
			ok, err := add_new_post_if_not_exist(blog, post)
			if err != nil {
				log.Fatal(err)
			}
			if ok {
				err = add_mail(blog, post)
				if err != nil {
					log.Fatal(err)
				}
			}
		}
	}
	// notify the user about the new sites
	notify()
}

func main() {
	// migrate()
	// list()
	// remove_site("localhost:6060/fakeblog")
	// list()
	// parseConfig()
	// explore("http://localhost:8000/fake-blog")
}

func main2() {
	// Parse command-line arguments
	migrateFlag := flag.Bool("migrate", false, "Create sqlite3 database and prepare tables")
	crawlFlag := flag.Bool("crawl", false, "Crawl web links")
	exploreFlag := flag.String("explore", "", "Add site to watchlist")
	listFlag := flag.Bool("list", false, "List saved sites")
	removeFlag := flag.String("remove", "", "Remove site from watchlist")

	flag.Parse()

	if *migrateFlag {
		migrate()
	}

	if *crawlFlag {
		run()
	}

	if *exploreFlag != "" {
		explore(*exploreFlag)
	}

	if *listFlag {
		sites, err := list_all_sites()
		if err != nil {
			log.Fatal(err)
		}
		for _, site := range sites {
			fmt.Println(site)
		}
	}

	if *removeFlag != "" {
		if err := remove_site(*removeFlag); err != nil {
			log.Fatal(err)
		}
	}
}

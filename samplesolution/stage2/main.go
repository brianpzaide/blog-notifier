package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

const (
	BLOGS_DB           = "./blogs.sqlite3"
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

// implements a functionality to remove a site
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

// updates the last visited site if new post in the blog site
func updateLastSiteVisited(site, link string) error {
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

func main() {

	// Parse command-line arguments
	migrateFlag := flag.Bool("migrate", false, "Create sqlite3 database and prepare tables")
	exploreFlag := flag.String("explore", "", "Add site to watchlist")
	listFlag := flag.Bool("list", false, "List saved sites")
	// lastLinkFlag := flag.Bool("lastLink", false, "update the last visited posts in a blog site")
	removeFlag := flag.String("remove", "", "Remove site from watchlist")

	updateFlag := flag.NewFlagSet("update-last-link", flag.ExitOnError)

	// Define multiple flags for the FlagSet
	var (
		flagBlogSite = updateFlag.String("site", "", "web address of the blog site")
		flagLastLink = updateFlag.String("post", "", "web address of the latest blog post")
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
		}

		if *exploreFlag != "" {
			if err := addNewSite(*exploreFlag, *exploreFlag); err != nil {
				fmt.Println(err.Error())
				return
			}
			fmt.Println("New blog added to watchlist:")
			fmt.Printf("site: %s\n", *exploreFlag)
			fmt.Printf("last link: %s\n", *exploreFlag)
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
			fmt.Println("For 'update-last-link' sub-command, 'site' and 'post' cannot be empty")
		}
	} else {
		fmt.Println("Invalid command")
	}
}

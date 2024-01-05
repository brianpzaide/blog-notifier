package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/PuerkitoBio/goquery"
)

const MAX_DEPTH = 3

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
func _crawl(link string, links *[]string, depth int) error {
	if depth > MAX_DEPTH {
		return nil
	}
	_links, err := findAllLinks(link)
	if err == nil {
		for _, _link := range _links {
			// fmt.Printf("%s at depth %d\n", _link, depth)
			*links = append(*links, _link)
			err := _crawl(_link, links, depth+1)
			if err != nil {
				return fmt.Errorf("%s: error in recursive crawl", link)
			}
		}
		return nil
	} else {
		return fmt.Errorf("%s: error in finAllLinks", link)
	}
}

// implements the crawl functionality
func crawl(blogSite string) ([]string, error) {
	// crawl the sites

	links := make([]string, 0)
	err := _crawl(blogSite, &links, 0)
	if err != nil {
		return nil, err
	} else {
		uniqueLinksMap := make(map[string]bool)
		uniqueLinks := make([]string, 0)
		for _, _link := range links {
			if _, ok := uniqueLinksMap[_link]; !ok {
				uniqueLinks = append(uniqueLinks, _link)
				uniqueLinksMap[_link] = true
			}
		}
		return uniqueLinks, nil
	}

}

func main() {
	// Parse command-line arguments
	crawlFlag := flag.String("crawlSite", "", "Crawl the given website")

	flag.Parse()

	if *crawlFlag != "" {
		fmt.Println("crawl")
		_links, err := crawl(*crawlFlag)
		if err != nil {
			log.Fatal(err)
		}
		for _, _link := range _links {
			fmt.Println(_link)
		}
	}
}

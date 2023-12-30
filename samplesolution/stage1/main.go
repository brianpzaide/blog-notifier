package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

const BLOGS_DB = "./blogs.sqlite3"

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

var conf blogNotifierConfig
var (
	mailAddr, sender, recipient, password string
)

// Parsing Config File  //
func parseConfig(configFile string) error {
	b, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	conf = blogNotifierConfig{}

	err = yaml.Unmarshal(b, &conf)
	if err != nil {
		fmt.Printf("error unmarshalling the config file %s", configFile)
		return err
	}
	mailAddr = fmt.Sprintf("%s:%d", conf.Server.Host, conf.Server.Port)
	sender, password, recipient = conf.Client.Email, conf.Client.Password, conf.Client.SendTo
	fmt.Printf("mode: %s\n", conf.Mode)
	fmt.Printf("email_server: %s\n", mailAddr)
	fmt.Printf("client: %s %s %s\n", sender, password, recipient)
	fmt.Printf("telegram: %s@%s\n", conf.Telegram.BotToken, conf.Telegram.Channel)
	return nil
}

func main() {
	// Parse command-line arguments
	parseFlag := flag.String("config", "", "parse the config file")

	flag.Parse()

	if *parseFlag != "" {
		err := parseConfig(*parseFlag)
		if err != nil {
			log.Fatal(err)
		}
	}
}

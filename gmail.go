// Copyright 2020 Albert "Drauthius" Diserholt. All rights reserved.
// Licensed under the MIT License.

// Get status for GMail. This requires some work on your part. Register a project on console.developers.google.com,
// then give access to that project to use GMail, and lastly create an OAuth 2.0 Client ID. The resulting JSON-file can be
// given as an argument to the program, and the first time you will be prompted with an URL to visit, and a token to
// fill in after logging in on the site.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/kirsle/configdir"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// Get a GMail service
func getService(config *oauth2.Config) *gmail.Service {
	ctx := context.Background()

	configDir := configdir.LocalConfig("oled-controller")
	err := configdir.MakePath(configDir)
	if err != nil {
		log.Printf("Failed to create configuration path %s: %v\n", configDir, err)
		return nil
	}
	tokenFile := filepath.Join(configDir, "token.json")
	token, err := getTokenFromFile(tokenFile)
	if err != nil {
		token := getTokenFromWeb(config)
		if token == nil {
			return nil
		}
		saveTokenToFile(tokenFile, token)
	}

	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(config.TokenSource(ctx, token)))
	if err != nil {
		log.Println("Failed to create GMail service:", err)
		return nil
	}

	return gmailService
}

// Get an API token from the web (2-peg OAuth or something)
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authUrl := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Please visit this URL to authenticate: \n%v\n", authUrl)

	fmt.Print("Code: ")
	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Println("Failed to read code:", err)
		return nil
	}

	token, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Println("Failed to exchange OAuth token:", err)
		return nil
	}
	return token
}

// Read a previously stored API token from a file.
func getTokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	token := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(token)
	return token, err
}

// Save a retrieved API token to a file.
func saveTokenToFile(file string, token *oauth2.Token) {
	fmt.Printf("Saving credentials to file '%s'\n", file)
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Println("Failed to cache OAuth token:", err)
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// Start a loop that gets the count of unread messages for a specific label.
func GmailStats(credentials string, label string, result chan int64, quit chan bool) {
	defer close(result)

	configContent, err := ioutil.ReadFile(credentials)
	if err != nil {
		log.Printf("Failed to read credentials file %s: %v\n", credentials, err)
		return
	}
	config, err := google.ConfigFromJSON(configContent, gmail.GmailLabelsScope)
	if err != nil {
		log.Println("Failed to create credentials from JSON:", err)
		return
	}

	gmailService := getService(config)
	if gmailService == nil {
		return
	}

	user := "me"
	for {
		label, err := gmailService.Users.Labels.Get(user, label).Do()
		if err != nil {
			log.Println("Failed to get unread message count:", err)
		} else {
			result <- label.MessagesUnread
		}

		select {
		case <-time.After(1 * time.Minute):
		case <-quit:
			return
		}
	}
}

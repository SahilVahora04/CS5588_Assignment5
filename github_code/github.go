package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"github.com/google/go-github/github"
)

// GitHubIssue represents the structure of a GitHub issue
type GitHubIssue struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

const (
	dbHost     = "assignment5-20502179:us-central1:mypostgres"
	dbPort     = 8080
	dbUser     = "mypostgres"
	dbPassword = "root"
	dbName     = "GitHubDB"
)

var (
	apiCalls = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "github_api_calls_total",
			Help: "Total number of GitHub API calls",
		},
		[]string{"repository"},
	)

	dataCollected = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "github_data_collected_total",
			Help: "Total amount of data collected from GitHub",
		},
		[]string{"repository"},
	)

	apiCallsPerSecond = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_api_calls_per_second",
			Help: "GitHub API calls per second",
		},
		[]string{"repository"},
	)

	dataCollectedPerSecond = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_data_collected_per_second",
			Help: "Data collected from GitHub per second",
		},
		[]string{"repository"},
	)
)

func init() {
	prometheus.MustRegister(apiCalls)
	prometheus.MustRegister(dataCollected)
	prometheus.MustRegister(apiCallsPerSecond)
	prometheus.MustRegister(dataCollectedPerSecond)
}

func saveGitHubIssuesToDatabase(issues []GitHubIssue, db *sql.DB, repository string) error {
	// Creating the issues table if it doesn't exist
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS github_issues (
			id SERIAL PRIMARY KEY,
			issue_id INT,
			title TEXT,
			body TEXT
		)
	`)
	if err != nil {
		return err
	}

	// Inserting each GitHub issue into the database
	for _, issue := range issues {
		_, err := db.Exec("INSERT INTO github_issues (issue_id, title, body) VALUES ($1, $2, $3)",
			issue.ID, issue.Title, issue.Body)
		if err != nil {
			return err
		}
	}

	apiCalls.WithLabelValues(repository).Inc()
	dataCollected.WithLabelValues(repository).Add(float64(len(issues)))

	return nil
}

func fetchGitHubIssues(repoURL, token string, duration time.Duration) ([]GitHubIssue, error) {
	// Parsing the repository owner and name from the URL
	parts := strings.Split(strings.TrimPrefix(repoURL, "https://github.com/"), "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid GitHub repository URL: %s", repoURL)
	}
	owner, repo := parts[0], parts[1]

	// Authenticating with the GitHub API using a personal access token
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	// Creating a GitHub client
	client := github.NewClient(tc)

	// Calculate the start date based on the specified duration
	startDate := time.Now().Add(-duration)

	// Fetching issues from the GitHub API within the specified time range
	issues, _, err := client.Issues.ListByRepo(context.Background(), owner, repo, &github.IssueListByRepoOptions{
		Since: startDate,
	})
	if err != nil {
		return nil, err
	}

	// Converting GitHub issues to the desired format
	var githubIssues []GitHubIssue
	for _, issue := range issues {
		githubIssues = append(githubIssues, GitHubIssue{
			ID:    *issue.ID,
			Title: *issue.Title,
			Body:  *issue.Body,
		})
	}

	return githubIssues, nil
}

func main() {
	// List of GitHub repository URLs
	githubURLs := []string{
		"https://github.com/prometheus/prometheus",
		"https://github.com/SeleniumHQ/selenium",
		"https://github.com/openai/openai-python",
		"https://github.com/docker/docs",
		"https://github.com/milvus-io/milvus",
		"https://github.com/golang/go",
	}

	token := "your_personal_access_token"

	// Establishing a database connection
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=require",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Error opening database connection:", err)
	}
	defer db.Close()

	// Serve Prometheus metrics
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	// Experiment durations in hours
	durations := []time.Duration{48 * time.Hour, 7 * 24 * time.Hour, 45 * 24 * time.Hour}

	// Iterate over GitHub URLs
	for _, githubURL := range githubURLs {
		// Iterate over durations
		for _, duration := range durations {
			// Fetching GitHub issues for the specified duration
			issues, err := fetchGitHubIssues(githubURL, token, duration)
			if err != nil {
				log.Printf("Error fetching GitHub issues for %s: %v", githubURL, err)
				continue
			}

			// Processing and storing GitHub issues as needed
			for _, issue := range issues {
				fmt.Printf("Repository: %s\nIssue ID: %d\nTitle: %s\nBody: %s\n\n", githubURL, issue.ID, issue.Title, issue.Body)
			}

			// Saving GitHub issues to the database
			err = saveGitHubIssuesToDatabase(issues, db, githubURL)
			if err != nil {
				log.Printf("Error saving GitHub issues to the database for %s: %v", githubURL, err)
			}

			// Calculate and set the API calls and data collected per second
			apiCallsPerSecond.WithLabelValues(githubURL).Set(apiCalls.WithLabelValues(githubURL).Value() / duration.Seconds())
			dataCollectedPerSecond.WithLabelValues(githubURL).Set(dataCollected.WithLabelValues(githubURL).Value() / duration.Seconds())
		}

		time.Sleep(1 * time.Minute)
	}

	select {}
}
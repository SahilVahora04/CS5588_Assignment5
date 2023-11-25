package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// StackOverflowResponse represents the structure of the Stack Overflow API response for questions and answers
type StackOverflowResponse struct {
	Items []struct {
		QuestionID   int    `json:"question_id"`
		Title        string `json:"title"`
		QuestionBody string `json:"question_body"`
		AnswerBody   string `json:"answer_body"`
	} `json:"items"`
}

const (
	dbHost     = "assignment5-20502179:us-central1:mypostgres"
	dbPort     = 8080
	dbUser     = "mypostgres"
	dbPassword = "root"
	dbName     = "StackoverflowDB"
)

var (
	apiCalls = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "stackoverflow_api_calls_total",
			Help: "Total number of Stack Overflow API calls",
		},
		[]string{"query"},
	)

	dataCollected = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "stackoverflow_data_collected_total",
			Help: "Total amount of data collected from Stack Overflow",
		},
		[]string{"query"},
	)

	apiCallsPerSecond = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "stackoverflow_api_calls_per_second",
			Help: "Stack Overflow API calls per second",
		},
		[]string{"query"},
	)

	dataCollectedPerSecond = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "stackoverflow_data_collected_per_second",
			Help: "Data collected from Stack Overflow per second",
		},
		[]string{"query"},
	)
)

func init() {
	prometheus.MustRegister(apiCalls)
	prometheus.MustRegister(dataCollected)
	prometheus.MustRegister(apiCallsPerSecond)
	prometheus.MustRegister(dataCollectedPerSecond)
}

func fetchStackOverflowData(url, queryLabel string, duration time.Duration) (*StackOverflowResponse, error) {
	apiCalls.WithLabelValues(queryLabel).Inc()

	// Calculating the start date based on the specified duration
	startDate := time.Now().Add(-duration)

	// Appending the start date to the URL query parameters
	url += fmt.Sprintf("&startdate=%d", startDate.Unix())

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	var stackOverflowData StackOverflowResponse

	// Extracting question and answer information
	doc.Find(".question-summary").Each(func(i int, s *goquery.Selection) {
		var item struct {
			QuestionID   int    `json:"question_id"`
			Title        string `json:"title"`
			QuestionBody string `json:"question_body"`
			AnswerBody   string `json:"answer_body"`
		}

		// Extracting question information
		href, exists := s.Find(".question-hyperlink").Attr("href")
		if exists {
			fmt.Sscanf(href, "/questions/%d", &item.QuestionID)
		}
		item.Title = s.Find(".question-hyperlink").Text()
		item.QuestionBody = s.Find(".excerpt").Text()

		// Fetching answer information by visiting the question page
		answerURL := fmt.Sprintf("https://stackoverflow.com%s", href)
		answerBody, err := fetchAnswerBody(answerURL)
		if err == nil {
			item.AnswerBody = answerBody
		}

		stackOverflowData.Items = append(stackOverflowData.Items, item)
	})

	dataCollected.WithLabelValues(queryLabel).Add(float64(len(stackOverflowData.Items)))

	return &stackOverflowData, nil
}

func fetchAnswerBody(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	// Extracting the answer body
	answerBody := doc.Find(".js-post-body").First().Text()

	return answerBody, nil
}

func saveToDatabase(data *StackOverflowResponse) error {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return err
	}
	defer db.Close()

	// Creating the questions table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS questions (
			id SERIAL PRIMARY KEY,
			question_id INT,
			title TEXT,
			question_body TEXT,
			answer_body TEXT
		)
	`)
	if err != nil {
		return err
	}

	// Inserting each question and answer into the database
	for _, item := range data.Items {
		_, err := db.Exec("INSERT INTO questions (question_id, title, question_body, answer_body) VALUES ($1, $2, $3, $4)",
			item.QuestionID, item.Title, item.QuestionBody, item.AnswerBody)
		if err != nil {
			return err
		}
	}

	return nil
}

func main() {
	// URLs to collect data from StackOverflow
	urls := []struct {
		URL   string
		Query string
	}{
		{"https://stackoverflow.com/search?q=Prometheus", "Prometheus"},
		{"https://stackoverflow.com/search?q=selenium-webdriver", "selenium-webdriver"},
		{"https://stackoverflow.com/search?q=OpenAi", "OpenAi"},
		{"https://stackoverflow.com/search?q=docker", "docker"},
		{"https://stackoverflow.com/search?q=milvus", "milvus"},
		{"https://stackoverflow.com/search?q=golang", "golang"},
	}

	// Prometheus metrics
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	// Experiment durations in hours
	durations := []time.Duration{48 * time.Hour, 7 * 24 * time.Hour, 45 * 24 * time.Hour}

	// Iterating over durations
	for _, duration := range durations {
		// Iterating over URLs
		for _, entry := range urls {
			stackOverflowData, err := fetchStackOverflowData(entry.URL, entry.Query, duration)
			if err != nil {
				log.Printf("Error fetching Stack Overflow data for %s: %v", entry.Query, err)
				continue
			}

			// Displaying question and answer information
			for _, item := range stackOverflowData.Items {
				fmt.Printf("Query: %s\nQuestion ID: %d\nTitle: %s\nQuestion Body: %s\nAnswer Body: %s\n\n", entry.Query, item.QuestionID, item.Title, item.QuestionBody, item.AnswerBody)
			}

			// Saving data to the database
			err = saveToDatabase(stackOverflowData)
			if err != nil {
				log.Printf("Error saving to database for %s: %v", entry.Query, err)
			}

			// metrics per second
			apiCallsPerSecond.WithLabelValues(entry.Query).Inc()
			dataCollectedPerSecond.WithLabelValues(entry.Query).Inc()			
				
		}

		time.Sleep(1 * time.Minute)
	}

	select {}
}
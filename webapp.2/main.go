package main

import (
	"encoding/csv"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

type serveValues struct {
	Excellent string
	Good      string
	Mediocre  string
	Bad       string
	Overall   string
	Success   string
	Primary   string
	Warning   string
	Alert     string
}

type feedback struct {
	Date     string
	Time     string
	Feedback string
}

type readings struct {
	Feedb string
	Val   int
}

var tpl *template.Template
var wg sync.WaitGroup

func showValues() map[string]int {

	config := &aws.Config{
		Region: aws.String("eu-west-1"),
		//Credentials: credentials,
		//HTTPClient: *http.Client,
	}
	sess := session.Must(session.NewSession(config))
	svc := dynamodb.New(sess)
	params := &dynamodb.ScanInput{
		TableName: aws.String("Feedback"),
	}
	result, err := svc.Scan(params)
	if err != nil {
		fmt.Println("Error while making scan request: ", err)
	}

	m := make(map[string]int)
	for _, i := range result.Items {
		item := readings{}

		err = dynamodbattribute.UnmarshalMap(i, &item)
		if err != nil {
			fmt.Println("Error while unmarshalling db items: ", err.Error())
		}
		m[item.Feedb] = item.Val
	}

	return m
}

func handleFeedback(s string) {

	timeNow := time.Now()
	f := feedback{
		Date:     timeNow.Format("02-01-2006"),
		Time:     timeNow.Format("15:04:05"),
		Feedback: s,
	}
	data := []string{f.Date, f.Time, f.Feedback}

	file, err := os.OpenFile("log.csv", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println("Error while opening log.csv", err)
		return
	}
	defer file.Close()

	w := csv.NewWriter(file)
	w.Write(data)
	defer w.Flush()

	fmt.Println(data)
}

func upItem(s string, m map[string]int, cs chan string) {

	var i int

	switch s {
	case "excellent":
		i = m["excellent"]
	case "good":
		i = m["good"]
	case "mediocre":
		i = m["mediocre"]
	case "bad":
		i = m["bad"]
	}
	config := &aws.Config{
		Region: aws.String("eu-west-1"),
		//Credentials: credentials,
		//HTTPClient: *http.Client,
	}

	sess := session.Must(session.NewSession(config))
	svc := dynamodb.New(sess)

	input := &dynamodb.UpdateItemInput{
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":r": {
				N: aws.String(strconv.Itoa(i + 1)),
			},
		},
		TableName: aws.String("Feedback"),
		Key: map[string]*dynamodb.AttributeValue{
			"feedb": {
				S: aws.String(s),
			},
		},
		ReturnValues:     aws.String("UPDATED_NEW"),
		UpdateExpression: aws.String("set val = :r"),
	}
	_, err := svc.UpdateItem(input)
	if err != nil {
		fmt.Println("Erroe while updating item to db: ", err.Error())
	}
	cs <- s
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	var p float64
	var sum float64

	var excellent float64
	var good float64
	var mediocre float64
	var bad float64

	m := showValues()
	cs := make(chan string)

	ex := float64(m["excellent"])
	gd := float64(m["good"])
	md := float64(m["mediocre"])
	bd := float64(m["bad"])

	switch r.FormValue("submit") {
	case "submit1":
		go upItem("excellent", m, cs)
		ex = ex + 1
	case "submit2":
		go upItem("good", m, cs)
		gd = gd + 1
	case "submit3":
		go upItem("mediocre", m, cs)
		md = md + 1
	case "submit4":
		go upItem("bad", m, cs)
		bd = bd + 1
	}

	sum = (ex + gd + md + bd)

	if sum == 0 {
		excellent = 0
		good = 0
		mediocre = 0
		bad = 0
	} else {
		p = sum / 100
		excellent = ex / p
		good = gd / p
		mediocre = md / p
		bad = bd / p
	}

	//convert and pass feedback values as strings to statistics.html
	pE := fmt.Sprintf("%.1f", excellent)
	pG := fmt.Sprintf("%.1f", good)
	pM := fmt.Sprintf("%.1f", mediocre)
	pB := fmt.Sprintf("%.1f", bad)
	overall := fmt.Sprintf("%.0f", sum)

	//convert and pass feedback values as strings to statistics.html js func
	ps1 := fmt.Sprintf("%.1f", excellent+2) + "%"
	ps2 := fmt.Sprintf("%.1f", good+2) + "%"
	ps3 := fmt.Sprintf("%.1f", mediocre+2) + "%"
	ps4 := fmt.Sprintf("%.1f", bad+2) + "%"

	v := serveValues{
		Excellent: pE,
		Good:      pG,
		Mediocre:  pM,
		Bad:       pB,
		Overall:   overall,
		Success:   ps1,
		Primary:   ps2,
		Warning:   ps3,
		Alert:     ps4,
	}
	go handleFeedback(<-cs)
	time.Sleep(time.Millisecond * 50)

	t, err := template.ParseFiles("templates/statistics.html")
	if err != nil {
		log.Println("Template parsing error: ", err)
	}
	err = t.Execute(w, v)
	if err != nil {
		log.Println("Error in template execution: ", err)
	}
}

func init() {
	tpl = template.Must(template.ParseGlob("templates/*.html"))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {

	err := tpl.ExecuteTemplate(w, "layout.html", nil)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	submit := r.FormValue("submit")

	if submit != "" {
		statsHandler(w, r)
	}
}

func main() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/statistics", statsHandler)
	log.Fatal(http.ListenAndServe(":8000", nil))
}

package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/gocolly/colly"
)

const (
	CHARTS_URL           string = "https://www.officialcharts.com/charts/singles-chart/%s"
	YOUTUBE_URL_TEMPLATE string = `<iframe width="560" height="315" src="{%s}"
	frameborder="0" allow="accelerometer; autoplay; encrypted-media; gyroscope;
	picture-in-picture" allowfullscreen></iframe>
	`
	YOUTUBE_QUERY_URL string = "https://www.youtube.com/results?search_query={%s} {%s}"
	numPos                   = 10
)

type chartPosition struct {
	Pos    int
	Artist string
	Song   string
	Url    string
}

func (curChartPosition *chartPosition) fillYoutubeClip(context context.Context) {

	var (
		href string
		ok   bool
	)

	url := fmt.Sprintf(YOUTUBE_QUERY_URL, curChartPosition.Artist, curChartPosition.Song)
	err := chromedp.Run(context,
		chromedp.Sleep(time.Second),
		chromedp.Navigate(url),
		chromedp.AttributeValue(`#video-title`, "href", &href, &ok, chromedp.ByQuery))
	if ok {
		curChartPosition.Url = strings.Replace(href, "/watch?v=", "", 1)
	}
	if err != nil {
		log.Fatal(err)
	}

}

type chartPositionArray [numPos]*chartPosition

var currentChart chartPositionArray

func main() {

	type templateData struct {
		CurrentChart chartPositionArray
	}

	if len(os.Args) < 2 {
		fmt.Println("No arguments. Please set necessary date ddmmYYYY format")
		os.Exit(1)
	}

	charDate := os.Args[1]
	timeChart, err := time.Parse("02012006", charDate)
	if err != nil {
		log.Fatalln(err)
	}

	getParse(timeChart)
	ctx, cancel := chromedp.NewContext(context.Background())
	for _, element := range currentChart {

		element.fillYoutubeClip(ctx)
	}
	defer cancel()
	f, err := os.OpenFile(fmt.Sprintf("%s.html", timeChart.Format("02012006")), os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	paths := []string{
		"chart_template.html",
	}
	t, err := template.ParseFiles(paths...)
	if err != nil {
		log.Fatal(err)
	}
	err = t.Execute(f, templateData{CurrentChart: currentChart})
	if err != nil {
		log.Fatal(err)
	}

}

func getParse(dateChart time.Time) {
	realURL := fmt.Sprintf(CHARTS_URL, dateChart.Format("20060102"))
	c := colly.NewCollector()
	num := 0
	c.OnHTML("div.title-artist", func(h *colly.HTMLElement) {
		if num < numPos {

			currentChart[num] = &chartPosition{
				Pos:    num + 1,
				Artist: h.ChildText(".artist"),
				Song:   h.ChildText(".title"),
			}
			num++
		}

	})
	c.Visit(realURL)
}

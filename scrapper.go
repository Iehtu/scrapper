package main

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
type fileDescr struct {
	FileName string
	Href     string
}

var currentChart chartPositionArray

func main() {

	// if len(os.Args) < 2 {
	// 	fmt.Println("No arguments. Please set necessary date ddmmYYYY format")
	// 	os.Exit(1)
	// }
	http.HandleFunc("/", getIndex)
	http.HandleFunc("/res", getResult)
	http.HandleFunc("/action", postAction)

	http.ListenAndServe(":5000", nil)
	//charDate := os.Args[1]
	//timeChart, err := time.Parse("02012006", charDate)

	// if err != nil {
	// 	log.Fatalln(err)
	// }

}

func postAction(w http.ResponseWriter, r *http.Request) {

	dateString := r.FormValue("curData")
	date, err := time.Parse("2006-01-02", dateString)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

	err = getHTMLChart(date)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}
	http.Redirect(w, r, "/", 307)
}
func getIndex(w http.ResponseWriter, r *http.Request) {

	files, err := readDirectory()
	if err != nil {
		fmt.Fprintln(w, "error "+err.Error())
	} else {
		t, err := template.ParseFiles("./templates/index.html")
		if err == nil {
			t.Execute(w, files)
		} else {
			fmt.Fprintln(w, err.Error())
		}
		// for _, file := range files {
		// 	fmt.Fprintln(w, file)
		// }
	}

}

func getResult(w http.ResponseWriter, r *http.Request) {

	p := r.URL.Query().Get("fileName")
	f, err := os.ReadFile("./result/" + p + ".html")
	if err != nil {
		http.Error(w, err.Error(), 500)
	} else {
		fmt.Fprint(w, string(f))
	}

}

func returnResult(file string) *fileDescr {
	href := file[len("result/"):]
	fileName := href[:len(href)-len(".html")]
	fileNameConv := fileName[:2] + "/" + fileName[2:4] + "/" + fileName[4:]
	hrefConv := "res?fileName=" + fileName
	return &fileDescr{Href: hrefConv, FileName: fileNameConv}
}

func readDirectory() ([]*fileDescr, error) {

	result := []*fileDescr{}
	err := filepath.WalkDir("./result/", func(path string, d fs.DirEntry, err error) error {

		if err != nil {
			return err
		}
		if filepath.Ext(d.Name()) == ".html" {
			result = append(result, returnResult(path))
		}

		return nil

	})
	if err != nil {
		return nil, err
	} else {
		return result, nil
	}
}

func getHTMLChart(timeChart time.Time) error {

	type templateData struct {
		CurrentChart chartPositionArray
	}

	log.Println("Запущена процедура чтения сайта")

	getParse(timeChart)
	ctx, cancel := chromedp.NewContext(context.Background())
	for _, element := range currentChart {
		log.Printf("Идет поиск клипа для позиции %d (%s-%s)\n", element.Pos, element.Artist, element.Song)
		element.fillYoutubeClip(ctx)
	}
	defer cancel()
	log.Println("Запись в файл")
	f, err := os.OpenFile("./result/"+fmt.Sprintf("%s.html", timeChart.Format("02012006")), os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		return err
	}
	defer f.Close()

	paths := []string{
		"./templates/chart_template.html",
	}
	t, err := template.ParseFiles(paths...)
	if err != nil {
		return err
	}
	err = t.Execute(f, templateData{CurrentChart: currentChart})
	if err != nil {
		return err
	}

	return nil
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

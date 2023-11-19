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
	"regexp"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/gocolly/colly"
)

const (
	CHARTS_URL           string = "https://www.officialcharts.com/charts/singles-chart/%s"
	CHARTS_URL_DE        string = "https://www.offiziellecharts.de/charts/single/for-date-%d"
	CHARTS_URL_US        string = "https://www.billboard.com/charts/hot-100/%s"
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

var re = regexp.MustCompile(`(?m)\/watch\?v=([^\&]+)`)

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
		href_short := re.FindStringSubmatch(href)
		if len(href_short) > 1 {
			curChartPosition.Url = href_short[1]
		}
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
	if len(os.Args) < 2 {
		mux := http.NewServeMux()

		log.Println("Server started at port 5000")
		mux.HandleFunc("/", getIndex)
		mux.HandleFunc("/res", getResult)
		mux.HandleFunc("/action", postAction)

		fileServerStyles := http.FileServer(http.Dir("./static"))
		mux.Handle("/static/", http.StripPrefix("/static", fileServerStyles))

		err := http.ListenAndServe(":5000", mux)
		log.Fatal(err)
	} else {
		charDate := os.Args[1]
		timeChart, err := time.Parse("02012006", charDate)
		if err != nil {

			log.Fatal(err)
		} else {
			country := "EN"
			if len(os.Args) > 2 {
				country = os.Args[2]

			}
			getHTMLChart(timeChart, country)
		}

	}
	//charDate := os.Args[1]
	//timeChart, err := time.Parse("02012006", charDate)

	// if err != nil {
	// 	log.Fatalln(err)
	// }

}

func postAction(w http.ResponseWriter, r *http.Request) {

	dateString := r.FormValue("curData")
	country := r.FormValue("country")
	date, err := time.Parse("2006-01-02", dateString)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

	log.Printf("Поиск чартов на дату: %s", date)
	err = getHTMLChart(date, country)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
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

func getHTMLChart(timeChart time.Time, country string) error {

	type templateData struct {
		CurrentChart chartPositionArray
		Title        string
	}

	log.Println("Запущена процедура чтения сайта")
	if country == "DE" {
		getParseDe(timeChart)
	} else if country == "US" {
		getParseUS(timeChart)
	} else {
		getParseEn(timeChart)
	}
	ctx, cancel := chromedp.NewContext(context.Background())
	for _, element := range currentChart {
		if element != nil {
			log.Printf("Идет поиск клипа для позиции %d (%s-%s)\n", element.Pos, element.Artist, element.Song)
			element.fillYoutubeClip(ctx)
		}
	}
	defer cancel()
	log.Println("Запись в файл")
	f, err := os.OpenFile("./result/"+fmt.Sprintf("%s_%s.html", timeChart.Format("02012006"), country), os.O_WRONLY|os.O_CREATE, 0777)
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
	err = t.Execute(f, templateData{CurrentChart: currentChart, Title: fmt.Sprintf("CHARTS(%s) %s", country, timeChart)})
	if err != nil {
		return err
	}

	return nil
}

func getParseEn(dateChart time.Time) {
	realURL := fmt.Sprintf(CHARTS_URL, dateChart.Format("20060102"))
	c := colly.NewCollector()
	num := 0
	c.OnHTML("div.description.block", func(h *colly.HTMLElement) {
		if num < numPos {
			currentChart[num] = &chartPosition{
				Pos:    num + 1,
				Artist: h.ChildText("a.chart-artist"),
				Song:   h.ChildText("a.chart-name"),
			}
			num++
		}

	})
	c.Visit(realURL)
}

func getParseDe(dateChart time.Time) {

	realURL := fmt.Sprintf(CHARTS_URL_DE, dateChart.UnixMilli())
	c := colly.NewCollector()
	num := 0
	c.OnHTML("tr.drill-down-link", func(h *colly.HTMLElement) {
		if num < numPos {
			currentChart[num] = &chartPosition{
				Pos:    num + 1,
				Artist: h.ChildText(".info-artist"),
				Song:   h.ChildText(".info-title"),
			}
			num++
		}

	})
	c.Visit(realURL)
}

func getParseUS(dateChart time.Time) {
	realURL := fmt.Sprintf(CHARTS_URL_US, dateChart.Format("2006-01-02"))
	c := colly.NewCollector()
	num := 0
	c.OnHTML("div.o-chart-results-list-row-container", func(h *colly.HTMLElement) {
		if num < numPos {
			currentChart[num] = &chartPosition{
				Pos:    num + 1,
				Artist: h.ChildText("span.c-label.u-letter-spacing-0021.u-max-width-330"),
				Song:   h.ChildText("h3#title-of-a-story.c-title.a-no-trucate.a-font-primary-bold-s.u-letter-spacing-0021"),
			}
			num++
		}

	})
	c.Visit(realURL)
}

package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"golang.org/x/net/html/charset"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	IntervalLength = 90
	UserAgent      = "tz_procontext"
)

type ValCurs struct {
	Date    string   `xml:"Date,attr"`
	Valutes []Valute `xml:"Valute"`
}

type Valute struct {
	NumCode   string  `xml:"NumCode"`
	CharCode  string  `xml:"CharCode"`
	Nominal   int     `xml:"Nominal"`
	Name      string  `xml:"Name"`
	Value     float64 `xml:"-"`
	VunitRate float64 `xml:"-"`
	ValueStr  string  `xml:"Value"`
	VunitStr  string  `xml:"VunitRate"`
	Date      string
}

// Запись данных о курсе валют
func fetchCurs(date string, wg *sync.WaitGroup, mu *sync.Mutex, valuteRecords map[string][]Valute) {
	defer wg.Done()

	url := fmt.Sprintf("http://www.cbr.ru/scripts/XML_daily_eng.asp?date_req=%s", date)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Error creating request for date %s: %v\n", date, err)
		return
	}
	req.Header.Set("user-agent", UserAgent)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error fetching data for date %s: %v\n", date, err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response for date %s: %v\n", date, err)
		return
	}

	var curs ValCurs
	reader := bytes.NewReader(body)
	decoder := xml.NewDecoder(reader)
	decoder.CharsetReader = charset.NewReaderLabel
	err = decoder.Decode(&curs)
	if err != nil {
		fmt.Printf("Error decoding XML for date %s: %v\n", date, err)
		return
	}

	for i, v := range curs.Valutes {
		v.Value, err = parseFloat(v.ValueStr)
		if err != nil {
			fmt.Printf("Error parsing Value %s: %v\n", v.ValueStr, err)
			return
		}
		v.VunitRate, err = parseFloat(v.VunitStr)
		if err != nil {
			fmt.Printf("Error parsing VunitRate %s: %v\n", v.VunitStr, err)
			return
		}
		v.Date = date
		curs.Valutes[i] = v
	}

	mu.Lock()
	for _, v := range curs.Valutes {
		valuteRecords[v.CharCode] = append(valuteRecords[v.CharCode], v)
	}
	mu.Unlock()
}

func main() {
	now := time.Now()
	startDate := now.AddDate(0, 0, -IntervalLength)

	valuteRecords := make(map[string][]Valute)

	var wg sync.WaitGroup
	var mu sync.Mutex

	for d := startDate; d.Before(now); d = d.AddDate(0, 0, 1) {
		date := d.Format("02/01/2006")
		wg.Add(1)
		go fetchCurs(date, &wg, &mu, valuteRecords)
	}
	wg.Wait()

	for charCode, rates := range valuteRecords {
		if len(rates) == 0 {
			continue
		}

		var minRate, maxRate, sumRate float64
		minRate = math.MaxFloat64
		maxRate = 0.0
		minRateDate := now.Format("02/01/2006")
		maxRateDate := startDate.Format("02/01/2006")

		for _, rate := range rates {
			if rate.Value < minRate {
				minRate = rate.Value
				minRateDate = rate.Date
			}
			if rate.Value > maxRate {
				maxRate = rate.Value
				maxRateDate = rate.Date
			}
			sumRate += rate.Value
		}

		// Среднее значение курса (исключая выходные и праздничные дни)
		avgRate := float64(sumRate) / float64(len(rates))

		fmt.Printf("%s(%s)\t, ", rates[0].Name, charCode)
		fmt.Printf("maximum value: %.4f on %s; ", maxRate, maxRateDate)
		fmt.Printf("minimum value: %.4f on %s; ", minRate, minRateDate)
		fmt.Printf("average value: %.4f\n", avgRate)
	}
}

func parseFloat(value string) (float64, error) {
	value = strings.Replace(value, ",", ".", -1)
	return strconv.ParseFloat(value, 64)
}

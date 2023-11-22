package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/peterbourgon/diskv/v3"
	"os"
	"strings"
	"time"
)

type lineTrace struct {
	Time time.Time
	Date string
}

type logLine struct {
	Time time.Time
	Text string
}

func main() {
	d := diskv.New(diskv.Options{
		BasePath:     os.Getenv("DB_FOLDER"),
		Transform:    func(s string) []string { return []string{} },
		CacheSizeMax: 1024 * 1024,
	})

	lineTrace, err := getLineTrace(d)
	if err != nil {
		panic(err)
	}

	date := time.Now().Format("20060102")

	if lineTrace.Date != date {
		lineTrace.Date = date
		lineTrace.Time = time.Now()
	}

	path := fmt.Sprintf("%s%s.log", os.Getenv("LOGS_FOLDER"), date)

	readFile, err := os.Open(path)
	if err != nil {
		panic(err)
	}

	fileScanner := bufio.NewScanner(readFile)
	fileScanner.Split(bufio.ScanLines)

	for fileScanner.Scan() {
		ll, err := lineToLogLine(fileScanner.Text())
		if err != nil {
			panic(err)
		}
		if ll.Time.Before(lineTrace.Time) {
			continue
		}

		// do actions here
		fmt.Println(fileScanner.Text())
		// end do action
		err = updateLineTrace(d, lineTrace)
		if err != nil {
			panic(err)
		}

	}

	readFile.Close()
}

func lineToLogLine(line string) (logLine, error) {
	lineSpl := strings.Split(line, "\t")

	tSpl := strings.Split(lineSpl[2], ".")[0]
	fmt.Println(tSpl)
	tFile, err := time.Parse("15:04:05", tSpl)
	if err != nil {
		return logLine{}, err
	}

	tNow := time.Now()

	return logLine{
		Time: time.Date(tNow.Year(), tNow.Month(), tNow.Day(), tFile.Hour(), tFile.Minute(), tFile.Second(), tFile.Nanosecond(), tNow.Location()),
		Text: line,
	}, nil
}

func getLineTrace(d *diskv.Diskv) (lineTrace, error) {
	b, err := d.Read("lineTrace")
	if err != nil {
		return lineTrace{}, err
	}

	var lt lineTrace
	err = json.Unmarshal(b, &lt)
	return lt, err
}

func updateLineTrace(d *diskv.Diskv, lt lineTrace) error {
	b, err := json.Marshal(lt)
	if err != nil {
		return err
	}

	return d.Write("lineTrace", b)
}

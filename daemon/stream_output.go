// Copyright 2024 Princess B33f Heavy Industries / Dave Shanley
// SPDX-License-Identifier: AGPL

package daemon

import (
	err2 "errors"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/pb33f/libopenapi-validator/errors"
	"github.com/pterm/pterm"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// minimumTimeUnit represents the smallest unit of time for rollover.
type minimumTimeUnit int

const (
	SECOND minimumTimeUnit = iota
	MINUTE
	HOUR
	DAY
	MONTH
	YEAR
)

// timeUnitMapping maps input strings to Go's time format strings and minimumTimeUnit.
type timeUnitMapping struct {
	input    string
	goFormat string
	unit     minimumTimeUnit
}

// timeUnitMappings defines the mapping from custom placeholders to Go's time format and time units.
// Ordered from largest to smallest to ensure correct replacement and unit determination.
var timeUnitMappings = []timeUnitMapping{
	{"YYYY", "2006", YEAR},
	{"YY", "06", YEAR},
	{"MM", "01", MONTH},
	{"DD", "02", DAY},
	{"HH", "15", HOUR},
	{"mm", "04", MINUTE},
	{"SS", "05", SECOND},
}

// listenForValidationErrors listens for validation errors and writes them to a report file.
func (ws *WiretapService) listenForValidationErrors() {
	ws.streamViolations = []*errors.ValidationError{}
	var lock sync.RWMutex
	json := jsoniter.ConfigCompatibleWithStandardLibrary

	f, rotateChan, err := ws.openReportFile()
	if err != nil {
		pterm.Error.Println("cannot stream violations: " + err.Error())
		return
	}

	go func() {
		defer f.Close()
		for {
			select {
			case violations := <-ws.streamChan:
				if ws.stream {
					lock.Lock()

					fi, _ := f.Stat()
					_ = os.Truncate(f.Name(), fi.Size()-1)
					if fi.Size() > 2 {
						_, _ = f.WriteString(",\n")
					}
					ws.streamViolations = append(ws.streamViolations, violations...)

					for i, v := range violations {
						bytes, _ := json.Marshal(v)
						if _, e := f.WriteString(fmt.Sprintf("%s", bytes)); e != nil {
							pterm.Error.Println("cannot write violation to stream:", e.Error())
						}
						if i < len(violations)-1 {
							_, _ = f.WriteString(",\n")
						}
					}
					_, _ = f.WriteString("]")
					lock.Unlock()
				}

			case <-rotateChan:
				f.Close()
				f, rotateChan, err = ws.openReportFile()
				if err != nil {
					pterm.Error.Println("Error rotating report file:", err.Error())
					return
				}
			}
		}
	}()
}

// openReportFile opens the report file with dynamic naming based on placeholders.
// It returns the opened file, a channel that signals when to rollover, and an error if any.
func (ws *WiretapService) openReportFile() (*os.File, <-chan time.Time, error) {
	// Regular expression to find placeholders within curly braces.
	placeholderRegex := regexp.MustCompile(`\{([^}]+)\}`)
	matches := placeholderRegex.FindStringSubmatch(ws.reportFile)

	var filename string
	var rolloverChan <-chan time.Time

	if len(matches) > 1 {
		pattern := matches[1]

		goTimeFormat, smallestUnit, err := convertPatternToGoTimeFormat(pattern)
		if err != nil {
			return nil, nil, err
		}

		filename = placeholderRegex.ReplaceAllString(ws.reportFile, time.Now().Format(goTimeFormat))

		nextRollover, err := calculateNextRollover(smallestUnit)
		if err != nil {
			// Keep the nil channel to prevent rollover.
			pterm.Error.Println("Error calculating next rollover:", err)
		} else {
			rolloverChan = time.After(nextRollover.Sub(time.Now()))
		}
	} else {
		filename = ws.reportFile
		_ = os.Remove(filename)
	}

	// Open the file.
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, err
	}

	fi, _ := f.Stat()
	if fi.Size() == 0 {
		if _, e := f.WriteString("[]"); err != nil {
			return nil, nil, e
		}
	}

	return f, rolloverChan, nil
}

// convertPatternToGoTimeFormat converts custom placeholders to Go's time format.
// It returns the Go time format string, the smallest time unit used, and an error if any.
func convertPatternToGoTimeFormat(pattern string) (string, minimumTimeUnit, error) {
	var smallestUnit minimumTimeUnit = YEAR // Initialize to the largest unit

	formatted := pattern

	for _, mapping := range timeUnitMappings {
		if strings.Contains(formatted, mapping.input) {
			// Replace all occurrences of the unit with its Go time format.
			formatted = strings.ReplaceAll(formatted, mapping.input, mapping.goFormat)
			smallestUnit = mapping.unit
		}
	}

	// Check if any replacement was made; if not, return an error.
	if formatted == pattern {
		return "", 0, err2.New("no valid time unit placeholders found in pattern")
	}

	return formatted, smallestUnit, nil
}

// calculateNextRollover calculates the next rollover time based on the smallest time unit.
func calculateNextRollover(smallestUnit minimumTimeUnit) (time.Time, error) {
	now := time.Now()

	switch smallestUnit {
	case SECOND:
		next := now.Truncate(time.Second).Add(time.Second)
		return next, nil
	case MINUTE:
		next := now.Truncate(time.Minute).Add(time.Minute)
		return next, nil
	case HOUR:
		next := now.Truncate(time.Hour).Add(time.Hour)
		return next, nil
	case DAY:
		// Truncate to the start of the day
		next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour)
		return next, nil
	case MONTH:
		year, month, _ := now.Date()
		location := now.Location()
		if month == 12 {
			year += 1
			month = 1
		} else {
			month += 1
		}
		next := time.Date(year, month, 1, 0, 0, 0, 0, location)
		return next, nil
	case YEAR:
		year, _, _ := now.Date()
		location := now.Location()
		year += 1
		next := time.Date(year, 1, 1, 0, 0, 0, 0, location)
		return next, nil
	default:
		return time.Time{}, err2.New("unknown smallest time unit")
	}
}

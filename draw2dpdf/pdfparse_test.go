// Copyright 2015 The draw2d Authors. All rights reserved.

package draw2dpdf_test

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// pdfPage describes a single page parsed from an uncompressed PDF
// document produced by gofpdf (see SetCompression(false)).
type pdfPage struct {
	mediaBox []float64
	content  string
}

// parsePdfPages extracts the MediaBox and the uncompressed content stream
// of every page in document order.
func parsePdfPages(pdf []byte) ([]pdfPage, error) {
	doc := string(pdf)
	objs := parseObjects(doc)
	rootBox, err := parseRootMediaBox(doc)
	if err != nil {
		return nil, err
	}
	// Locate the Pages root and its ordered Kids.
	rootRe := regexp.MustCompile(`(?s)<<\s*/Type\s*/Pages(.*?)>>\s*endobj`)
	root := rootRe.FindStringSubmatch(doc)
	if root == nil {
		return nil, errors.New("pages root not found")
	}
	kidsRe := regexp.MustCompile(`(\d+) 0 R`)
	kids := kidsRe.FindAllStringSubmatch(root[1], -1)
	pages := make([]pdfPage, 0, len(kids))
	for _, kid := range kids {
		dict, ok := objs[kid[1]]
		if !ok {
			return nil, errors.New("page object " + kid[1] + " not found")
		}
		page := pdfPage{mediaBox: rootBox}
		if mb := regexp.MustCompile(`/MediaBox\s*\[([^\]]+)\]`).FindStringSubmatch(dict); mb != nil {
			page.mediaBox = parseFloats(mb[1])
		}
		contentsRe := regexp.MustCompile(`/Contents\s+(\d+) 0 R`)
		cm := contentsRe.FindStringSubmatch(dict)
		if cm == nil {
			return nil, errors.New("page " + kid[1] + " has no contents")
		}
		content, err := parseStream(objs[cm[1]])
		if err != nil {
			return nil, err
		}
		page.content = content
		pages = append(pages, page)
	}
	return pages, nil
}

// parseRootMediaBox reads the default MediaBox from the Pages root.
func parseRootMediaBox(doc string) ([]float64, error) {
	rootRe := regexp.MustCompile(`(?s)<<\s*/Type\s*/Pages.*?/MediaBox\s*\[([^\]]+)\].*?>>\s*endobj`)
	m := rootRe.FindStringSubmatch(doc)
	if m == nil {
		return nil, errors.New("root MediaBox not found")
	}
	return parseFloats(m[1]), nil
}

// parseObjects splits a PDF file into indirect object bodies keyed by
// object number. Object boundaries are the "N 0 obj" markers, so nested
// dictionaries do not confuse the parsing.
func parseObjects(doc string) map[string]string {
	objs := map[string]string{}
	marker := regexp.MustCompile(`(?m)^(\d+) 0 obj\s*$`)
	matches := marker.FindAllStringSubmatchIndex(doc, -1)
	for i, m := range matches {
		num := doc[m[2]:m[3]]
		bodyStart := m[1]
		var bodyEnd int
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0]
		} else {
			bodyEnd = len(doc)
		}
		objs[num] = doc[bodyStart:bodyEnd]
	}
	return objs
}

func parseStream(body string) (string, error) {
	start := strings.Index(body, "stream")
	if start < 0 {
		return "", errors.New("stream keyword not found")
	}
	start += len("stream")
	if start < len(body) && body[start] == '\r' {
		start++
	}
	if start < len(body) && body[start] == '\n' {
		start++
	}
	end := strings.Index(body[start:], "endstream")
	if end < 0 {
		return "", errors.New("endstream keyword not found")
	}
	return body[start : start+end], nil
}

func parseFloats(s string) []float64 {
	fields := regexp.MustCompile(`\S+`).FindAllString(s, -1)
	out := make([]float64, 0, len(fields))
	for _, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err == nil {
			out = append(out, v)
		}
	}
	return out
}

// countOccurrences returns how often substr occurs in s.
func countOccurrences(s, substr string) int {
	return strings.Count(s, substr)
}

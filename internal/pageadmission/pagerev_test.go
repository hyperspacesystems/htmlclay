package pageadmission

import "testing"

func TestPageRev(t *testing.T) {
	for _, test := range []struct {
		name, source, want string
	}{
		{"one revisioned article", oldFormatDocument(`<article class="blockeditor" data-blockeditor="1" data-rev="r1"></article>`), "r1"},
		{"an article with no data-rev", oldFormatDocument(`<article data-blockeditor="1"></article>`), ""},
		{"two articles", oldFormatDocument(`<article data-blockeditor="1" data-rev="a"></article><article data-blockeditor="2" data-rev="b"></article>`), ""},
		{"an article inside a template", oldFormatDocument(`<template><article data-blockeditor="1" data-rev="t"></article></template>`), ""},
		{"an article inside a noscript beside a live one", oldFormatDocument(`<noscript><article data-blockeditor="1" data-rev="n"></article></noscript><article data-blockeditor="1" data-rev="live"></article>`), "live"},
		{"a pagination article", oldFormatDocument(`<article data-page="2" data-rev="p"></article>`), ""},
		{"odd case", oldFormatDocument(`<ARTICLE DATA-BLOCKEDITOR="1" DATA-REV="Up"></ARTICLE>`), "Up"},
		{"empty", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := PageRev(test.source); got != test.want {
				t.Fatalf("PageRev = %q, want %q", got, test.want)
			}
		})
	}
}

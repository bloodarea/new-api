package controller

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseIconFromHTML(t *testing.T) {
	base, err := url.Parse("https://example.com/some/page")
	require.NoError(t, err)

	cases := []struct {
		name string
		html string
		want string
	}{
		{
			name: "relative icon href is resolved",
			html: `<html><head><link rel="icon" href="/static/favicon.png"></head></html>`,
			want: "https://example.com/static/favicon.png",
		},
		{
			name: "href before rel still matches",
			html: `<link href="favicon.ico" rel="shortcut icon">`,
			want: "https://example.com/some/favicon.ico",
		},
		{
			name: "protocol relative href keeps page scheme",
			html: `<link rel="icon" href="//cdn.example.com/i.ico">`,
			want: "https://cdn.example.com/i.ico",
		},
		{
			name: "apple touch icon used when no regular icon",
			html: `<link rel="apple-touch-icon" href="/apple.png">`,
			want: "https://example.com/apple.png",
		},
		{
			name: "regular icon wins over apple touch icon",
			html: `<link rel="apple-touch-icon" href="/apple.png"><link rel="icon" href="/icon.svg">`,
			want: "https://example.com/icon.svg",
		},
		{
			name: "no icon link returns empty",
			html: `<html><head><title>x</title></head></html>`,
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseIconFromHTML(tc.html, base))
		})
	}
}

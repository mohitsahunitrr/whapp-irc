package main

import (
	"encoding/base64"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"regexp"
	"strconv"
	"time"

	"github.com/h2non/filetype"
	"github.com/mozillazg/go-unidecode"
)

func strTimestamp() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

func getExtension(bytes []byte) string {
	typ, err := filetype.Match(bytes)
	if err != nil {
		return ""
	}

	res := typ.Extension
	if res == "unknown" {
		return ""
	}
	return res
}

func getExtensionByMime(typ string) (string, error) {
	extensions, err := mime.ExtensionsByType(typ)
	if err != nil {
		return "", err
	}

	if len(extensions) == 0 {
		return "", nil
	}

	return extensions[0][1:], nil
}

func getExtensionByMimeOrBytes(mime string, bytes []byte) string {
	if res, err := getExtensionByMime(mime); res != "" && err == nil {
		return res
	}

	return getExtension(bytes)
}

var unsafeRegex = regexp.MustCompile(`(?i)[^a-z\d+]`)

func ircSafeString(str string) string {
	str = unidecode.Unidecode(str)
	return unsafeRegex.ReplaceAllLiteralString(str, "")
}

func onInterrupt(fn func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		fn()
		os.Exit(1)
	}()
}

func b64tob64url(str string) (string, error) {
	bytes, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func b64urltob64(str string) (string, error) {
	bytes, err := base64.RawURLEncoding.DecodeString(str)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

func noDirListing(handler http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if path.Clean(r.URL.Path) == "/" {
			http.NotFound(w, r)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

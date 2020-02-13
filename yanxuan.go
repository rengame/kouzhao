package main

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
)

//通过平台API查询库存
func checkYanXuanStockByAPI(ctx context.Context, URL string, ch chan bool) {
	defer func() {
		luNum--
		ch <- true
	}()
	luNum++

	var (
		err  error
		req  *http.Request
		resp *http.Response
	)

	if req, err = http.NewRequest(`GET`, URL, nil); err != nil {
		log.Println(err)
		return
	}

	if resp, err = http.DefaultClient.Do(req); err != nil {
		log.Println(err)
		return
	}

	defer resp.Body.Close()
	var reader io.Reader

	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, _ = gzip.NewReader(resp.Body)
	default:
		reader = resp.Body
	}

	returnStr, _ := ioutil.ReadAll(reader)

	flysnowRegexp := regexp.MustCompile(`"soldOut":(true|false).+"status":(\d)`)
	params := flysnowRegexp.FindStringSubmatch(string(returnStr))

	if len(params) > 2 && params[1] == "false" && params[2] != "0" {
		fmt.Println(URL, "有货")
		noticeByBark(URL)
	} else {
		fmt.Println(URL, "无货")
	}
}

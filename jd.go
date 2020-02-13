package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/axgle/mahonia"
)

const (
	//JDStockStateAPI url
	JDStockStateAPI = "https://c0.3.cn/stocks"
	//areaID
	areaID = "2_2834_51988_0"
)

type stockStateStruct struct {
	Name  string `json:"StockStateName"`
	State int    `json:"skuState"`
}

//通过平台API查询库存
func checkJDStockByAPI(ctx context.Context, URL string, ch chan bool) {
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
	flysnowRegexp := regexp.MustCompile(`^https://item.jd.com/(\d+).html`)
	params := flysnowRegexp.FindStringSubmatch(URL)

	u, _ := url.Parse(JDStockStateAPI)
	q := u.Query()
	q.Set("type", "getstocks")
	q.Set("skuIds", params[1])
	q.Set("area", areaID)
	q.Set("_", strconv.FormatInt(time.Now().Unix()*1000, 10))

	u.RawQuery = q.Encode()
	queryURL := u.String()
	//fmt.Println(queryURL)

	if req, err = http.NewRequest(`GET`, queryURL, nil); err != nil {
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
	dec := mahonia.NewDecoder("gbk")
	decString := dec.ConvertString(string(returnStr))
	var dat map[string]stockStateStruct
	if err := json.Unmarshal([]byte(decString), &dat); err != nil {
		log.Println("转换JSON失败")
		return
	}

	for _, val := range dat {
		//log.Println(val.State, val.Name)
		if val.State == 1 && val.Name == "现货" {
			log.Println(URL, "有货")
			noticeByBark(URL)
		} else {
			log.Println(URL, "无货")
		}
	}
}

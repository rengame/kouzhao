package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"time"
)

const (
	//suningAreaID
	suningAreaID        = "20_021_0210199"
	suningAPIURL string = "https://pas.suning.com/nsenitemsale_%s_0000000000_5_999_%s.html"
)

type sunningStockStateStruct struct {
	Name  string `json:"StockStateName"`
	State int    `json:"skuState"`
}

type suningItemInfoForRoot struct {
	Data suningItemInfoForData `json:"data"`
}

type suningItemInfoForData struct {
	Data1     suningItemInfoForData1 `json:"data1"`
	InvStatus string                 `json:"invStatus"`
	Price     map[string]interface{} `json:"price"`
}

type suningItemInfoForData1 struct {
	Data suningItemInfoForData2 `json:"data"`
}

type suningItemInfoForData2 struct {
	ItemInfoVo suningItemInfoForItem `json:"itemInfoVo"`
}

type suningItemInfoForItem struct {
	Published string `json:"published"`
}

//通过平台API查询库存
func checkSuningStockByAPI(ctx context.Context, URL string, ch chan bool) {
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
	matchRegexp := regexp.MustCompile(`^https://m.suning.com/product/(\d+).html`)
	params := matchRegexp.FindStringSubmatch(URL)

	queryURL := fmt.Sprintf(suningAPIURL, params[1], suningAreaID)

	//q.Set("_", strconv.FormatInt(time.Now().Unix()*1000, 10))
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

	matchRegexp = regexp.MustCompile(`wapData\((.+)\)`)
	matchArr := matchRegexp.FindStringSubmatch(string(returnStr))

	if len(matchArr) < 2 {
		log.Println(URL, "wapData匹配失败")
		return
	}

	var dat suningItemInfoForRoot
	if err = json.Unmarshal([]byte(matchArr[1]), &dat); err != nil {
		log.Println("转换JSON失败")
		return
	}

	invStatus := dat.Data.InvStatus
	published := dat.Data.Data1.Data.ItemInfoVo.Published

	fmt.Println(URL, invStatus, published)

	if published == "1" && invStatus == "1" {
		fmt.Println(URL, "有货")
		noticeByBark(URL)
	}
	time.Sleep(300 * time.Millisecond)

	// var dat map[string]stockStateStruct
	// if err := json.Unmarshal([]byte(returnStr), &dat); err != nil {
	// 	log.Println("转换JSON失败")
	// 	return
	// }

	// for _, val := range dat {
	// 	//log.Println(val.State, val.Name)
	// 	if val.State == 1 && val.Name == "现货" {
	// 		log.Println(URL, "有货")
	// 	} else {
	// 		log.Println(URL, "无货")
	// 	}
	// }
}

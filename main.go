package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/axgle/mahonia"
	"github.com/chromedp/chromedp"
)

const (
	//JDStockStateAPI url
	JDStockStateAPI = "https://c0.3.cn/stocks"
	//ShangHai
	areaID = "2_2834_51988_0"
	//BarkNoticeAPI 自己的barkApi链接，Appstore下载Bark得到
	BarkNoticeAPI = "https://api.day.app/"
	//TaskTimeout chrome 作务超时时间，秒
	TaskTimeout = 10
)

type stockStateStruct struct {
	Name  string `json:"StockStateName"`
	State int    `json:"StockState"`
}

type shopInfo struct {
	Name     string
	Keywords []string
	Selector string
	Urls     []string
}

var (
	runnerNum int //并发数
	luNum     int //实际Chrome线程数
)

var barkToken = flag.String("token", "", "bark分配的token")

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

//工作线程，Chromedp的多次run如果共用一个Context,是可以在一个进程上反复fetch，否则一次run就要重启一个进程，效率虽低，但是可以解决timeout问题，也避免因频率太高，被Ban
func run(timeout int, taskChan chan map[string]string) {
	//ctx, cancel := chromedp.NewContext(context.Background())
	defer func() {
		runnerNum--
	}()
	runnerNum++
	for taskInfo := range taskChan {
		//缓冲为1，避免goroutine溢出
		taskCompleted := make(chan bool, 1)
		//超时结束Chromedp进程，官方文档没找到timeout相关操作
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()
		ctx, cancel = chromedp.NewContext(ctx)
		defer cancel()

		go checkStock(ctx, taskInfo, taskCompleted)
		//go checkJDStockByAPI(url, areaID, chrun)
		select {
		case <-taskCompleted:
		//case <-time.After(time.Duration(timeout) * time.Second):
		case <-ctx.Done():
			log.Println(taskInfo["goid"], "timeout:", taskInfo["URL"])
			//cancel()
		}
	}
}

//通过平台API查询库存
func checkJDStockByAPI(URL string, areaID string, ch chan bool) {
	defer func() { ch <- true }()

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
	fmt.Println(queryURL)

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
		log.Println(val.Name)
	}
}

func checkStock(ctx context.Context, taskInfo map[string]string, ch chan bool) {
	defer func() {
		luNum--
		ch <- true
	}()
	luNum++
	var responseStr string
	keywords := strings.Split(taskInfo["Keywords"], ",")
	err := chromedp.Run(
		ctx,
		chromedp.Navigate(taskInfo["URL"]),
		chromedp.TextContent(taskInfo["Selector"], &responseStr, chromedp.NodeReady, chromedp.ByQueryAll),
	)

	if err != nil {
		log.Println(taskInfo["goid"], err)
		return
	}

	if len(responseStr) == 0 {
		log.Println("reponse content len:0")
		return
	}

	matchState := 0

	for _, keyword := range keywords {
		if strings.Contains(responseStr, keyword) {
			matchState++
		}
	}

	if matchState == 0 && len(responseStr) > 0 {
		log.Printf("goid:%s,len:%d,content:%s", taskInfo["goid"], len(responseStr), responseStr)
		log.Println(taskInfo["goid"], taskInfo["URL"], `有货`)
		noticeURL := BarkNoticeAPI + *barkToken + "/KouZhao?url=" + taskInfo["URL"]
		req, _ := http.NewRequest("GET", noticeURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Println(err)
			return
		}
		defer resp.Body.Close()
		body, _ := ioutil.ReadAll(resp.Body)
		log.Println(string(body))
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	flag.Parse()

	runnerNum = 0
	luNum = 0

	if len(*barkToken) == 0 {
		log.Fatalln("Missing token parameter")
	}

	var shopConfig []shopInfo
	file, err := os.Open("conf/shop.json")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&shopConfig)
	if err != nil {
		log.Panicln("Error:", err)
	}

	c := make(chan map[string]string, runtime.NumCPU())

	//用于查看goroutine堆栈等信息，http://localhost:9999/debug/pprof/
	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:9999", nil))
	}()

	t := time.Tick(time.Second * 2)
	go func() {
		for {
			select {
			case <-t:
				log.Printf("NumGoroutine: %d,chan len:%d,runNum:%d,luNum:%d\n", runtime.NumGoroutine(), len(c), runnerNum, luNum)
			}
		}
	}()

	for i := 0; i < runtime.NumCPU(); i++ {
		go run(TaskTimeout, c)
	}

	for true {
		for skey, val := range shopConfig {
			for ukey, url := range val.Urls {
				//模拟一个goroutineID，利于调试
				goid := strconv.Itoa(skey) + "-" + strconv.Itoa(ukey)
				taskInfo := map[string]string{
					"Keywords": strings.Join(val.Keywords, ","),
					"Selector": val.Selector,
					"URL":      url,
					"goid":     goid,
				}
				c <- taskInfo
			}
		}
	}

	close(c)
}

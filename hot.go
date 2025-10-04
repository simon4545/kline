package main

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/parnurzeal/gorequest"
	"github.com/remeh/sizedwaitgroup"
	"github.com/tidwall/gjson"
)

var binanceExcludes []string = []string{"USDC", "FDUSD", "TUSD", "USDP", "FDUSD", "AEUR", "ASR", "OG", "WNXM", "WBETH", "WBTC",
	"WAXP", "FOR", "JST", "SUN", "WIN", "TRX", "UTK", "TROY", "WRX", "DOCK", "C98", "EUR", "USTC", "USDS", "AUD", "DAI"}

var DOWNUP []string = []string{"DOWN", "UP"}
var request *gorequest.SuperAgent

type KLine struct {
	Symbol    string
	Price     float64
	TimeStamp int64
}
type HotPair struct {
	Symbol      string
	LastPrice   float64
	Percent     float64
	QuoteVolume float64
	Klines      []KLine
}
type HotPairList []*HotPair

func (ps HotPairList) Len() int {
	return len(ps)
}

func (ps HotPairList) Less(i, j int) bool {
	return ps[i].Percent > ps[j].Percent
}

func (ps HotPairList) Swap(i, j int) {
	ps[i], ps[j] = ps[j], ps[i]
}
func ContainsString(slice []string, target string) bool {
	for _, v := range slice {
		if v == target {
			return true
		}
	}
	return false
}
func HotList() (symols HotPairList) {
	request = gorequest.New()
	url := "https://fapi.binance.com/fapi/v1/ticker/24hr"
	_, responseBody, err := request.Get(url).End()
	if err != nil {
		log.Fatal("Error making GET request:", err)
	}

	value := gjson.Parse(responseBody).Array()
	for _, symbol := range value {
		symbolCoin := symbol.Get("symbol").String()
		volume24h := symbol.Get("quoteVolume").Float()
		lastPrice := symbol.Get("lastPrice").Float()
		if volume24h > 5_000_000 && strings.Contains(symbolCoin, "USDT") {
			baseAsset := symbolCoin[:len(symbolCoin)-4]
			quoteAsset := symbolCoin[len(symbolCoin)-4:]
			if quoteAsset == "USDT" && !ContainsString(binanceExcludes, baseAsset) && !strings.HasSuffix(baseAsset, "DOWN") && !strings.HasSuffix(baseAsset, "UP") {
				priceChangePercent := symbol.Get("priceChangePercent").Float()
				pair := HotPair{Symbol: baseAsset, LastPrice: lastPrice, QuoteVolume: volume24h, Percent: priceChangePercent}
				symols = append(symols, &pair)
			}
		}
	}
	sort.Sort(symols)
	symols = symols[:35]
	swg := sizedwaitgroup.New(4)
	for _, s := range symols {
		swg.Add()
		go func(s *HotPair) {
			defer swg.Done()
			list := CollectTrendWithSymbol(s.Symbol, "5m")
			s.Klines = list
			// time.Sleep(time.Millisecond * 200)
		}(s)
	}
	swg.Wait()

	return
}
func CollectTrendWithSymbol(pair string, interval string) (klines []KLine) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/klines?symbol=%sUSDT&interval=%s&limit=%d", pair, interval, 50)
	_, bodyBytes, err := request.Get(url).End()
	// response, err := http.Get(url)
	if err != nil {
		log.Fatal("Error making GET request:", err)
	}
	// defer response.Body.Close()
	// bodyBytes, err := io.ReadAll(response.Body)
	// if err != nil {
	// 	log.Fatal("Error reading response body:", err)
	// }
	value := gjson.Parse(string(bodyBytes)).Array()
	if len(value) > 0 {
		for _, v := range value {
			c, _ := strconv.ParseFloat(v.Array()[4].String(), 64)
			t, _ := strconv.ParseInt(v.Array()[6].String(), 10, 64)
			klines = append(klines, KLine{Symbol: pair, Price: c, TimeStamp: t})
		}
	}
	return
}

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
)

// APIResponse 定义API响应结构
type APIResponse struct {
	Success bool       `json:"success"`
	Code    string     `json:"code"`
	Data    DataObject `json:"data"`
}

// DataObject 包含列表数据
type DataObject struct {
	List       []CoinData `json:"list"`
	Pagination Pagination `json:"pagination"`
}

// Pagination 分页信息
type Pagination struct {
	Current  int `json:"current"`
	Total    int `json:"total"`
	PageSize int `json:"pageSize"`
}

// CoinData API返回的币种数据
type CoinData struct {
	BaseCoin           string  `json:"baseCoin"`
	Price              float64 `json:"price"`
	PriceChangeM5      float64 `json:"priceChangeM5"`
	PriceChangeM15     float64 `json:"priceChangeM15"`
	PriceChangeM30     float64 `json:"priceChangeM30"`
	OpenInterestChM5   float64 `json:"openInterestChM5"`
	OpenInterestChM15  float64 `json:"openInterestChM15"`
	LongShortRatio     float64 `json:"longShortRatio"`
	FundingRate        float64 `json:"fundingRate"`
	Buy5m              float64 `json:"buy5m"`
	Sell5m             float64 `json:"sell5m"`
	Buy15m             float64 `json:"buy15m"`
	Sell15m            float64 `json:"sell15m"`
	Buy24h             float64 `json:"buy24h"`
	Sell24h            float64 `json:"sell24h"`
	RsiM5              float64 `json:"rsiM5"`
	RsiM15             float64 `json:"rsiM15"`
	LiquidationH1      float64 `json:"liquidationH1"`
	LiquidationH1Long  float64 `json:"liquidationH1Long"`
	LiquidationH1Short float64 `json:"liquidationH1Short"`
	VolatilityM5       float64 `json:"volatilityM5"`
	VolatilityM15      float64 `json:"volatilityM15"`
}

// PredictionRecord 预测记录结构
type PredictionRecord struct {
	Timestamp  time.Time          // 预测时间
	Price      float64            // 当时的价格
	Score      float64            // 预测分数
	Indicators map[string]float64 // 主要指标值
}

// MarketAnalyzer 市场分析器
type MarketAnalyzer struct {
	// 各指标权重配置
	weights map[string]float64
	// 其他配置参数
	config map[string]interface{}
	// 历史预测记录
	predictionHistory []PredictionRecord
	// 锁，保护历史记录的并发访问
	historyMutex sync.Mutex
	// 胜率统计
	winCount   int
	totalCount int
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	// 创建分析器
	analyzer := NewMarketAnalyzer()

	// 设置信号处理，用于优雅退出
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	c := cron.New(cron.WithSeconds())
	// 启动定时任务
	executeCalculation(analyzer)
	c.AddFunc("5 * * * * *", func() {
		executeCalculation(analyzer)
	})
	c.Start()

	// 等待退出信号
	<-sigs
	log.Println("接收到退出信号，正在关闭服务...")
}

// executeCalculation 执行一次计算并输出结果
func executeCalculation(analyzer *MarketAnalyzer) {
	log.Println("开始执行计算...")

	// 获取数据
	data, err := analyzer.FetchCoinData()
	if err != nil {
		log.Printf("获取数据失败: %v\n", err)
		return
	}

	// 计算趋势分数
	score := analyzer.CalculateTrendScore(data)

	// 获取趋势摘要
	summary := analyzer.GetTrendSummary(data)

	now := time.Now()

	// 输出结果
	log.Printf("BTC价格: %.2f\n", data.Price)
	log.Printf("趋势分数: %.1f\n", score)
	log.Printf("趋势评价: %s\n", summary)

	// 输出详细指标贡献
	priceScore := analyzer.calculatePriceScore(data)
	flowScore := analyzer.calculateFlowScore(data)
	sentimentScore := analyzer.calculateSentimentScore(data)
	technicalScore := analyzer.calculateTechnicalScore(data)
	liquidationScore := analyzer.calculateLiquidationScore(data)

	log.Println("详细指标贡献:")
	log.Printf("价格动量: %.1f/100\n", priceScore)
	log.Printf("资金流向: %.1f/100\n", flowScore)
	log.Printf("多空情绪: %.1f/100\n", sentimentScore)
	log.Printf("技术指标: %.1f/100\n", technicalScore)
	log.Printf("清算压力: %.1f/100\n", liquidationScore)

	// 输出原始指标值
	// log.Printf("5分钟价格变化: %.2f%%\n", data.PriceChangeM5)
	// log.Printf("15分钟价格变化: %.2f%%\n", data.PriceChangeM15)
	// log.Printf("30分钟价格变化: %.2f%%\n", data.PriceChangeM30)
	// log.Printf("多空比: %.4f\n", data.LongShortRatio)
	// log.Printf("资金费率: %.6f\n", data.FundingRate)
	// log.Printf("5分钟RSI: %.2f\n", data.RsiM5)
	// log.Printf("15分钟RSI: %.2f\n", data.RsiM15)
	// log.Printf("5分钟波动率: %.6f\n", data.VolatilityM5)
	// log.Printf("15分钟波动率: %.6f\n\n", data.VolatilityM15)

	// 创建并保存预测记录
	record := PredictionRecord{
		Timestamp: now,
		Price:     data.Price,
		Score:     score,
		Indicators: map[string]float64{
			"PriceChangeM5":  data.PriceChangeM5,
			"PriceChangeM15": data.PriceChangeM15,
			"PriceChangeM30": data.PriceChangeM30,
			"LongShortRatio": data.LongShortRatio,
			"FundingRate":    data.FundingRate,
			"RsiM5":          data.RsiM5,
			"RsiM15":         data.RsiM15,
			"VolatilityM5":   data.VolatilityM5,
			"VolatilityM15":  data.VolatilityM15,
		},
	}

	// 检查10分钟前的预测结果，计算胜率
	analyzer.checkPastPrediction(data, now)

	// 保存当前记录
	analyzer.savePredictionRecord(record)

	// 输出当前胜率
	if analyzer.totalCount > 0 {
		winRate := float64(analyzer.winCount) / float64(analyzer.totalCount) * 100
		log.Printf("当前预测胜率: %.1f%% (正确: %d, 总计: %d)\n",
			winRate, analyzer.winCount, analyzer.totalCount)
	} else {
		log.Println("尚无足够历史数据计算胜率\n")
	}
}

// savePredictionRecord 保存预测记录
func (a *MarketAnalyzer) savePredictionRecord(record PredictionRecord) {
	a.historyMutex.Lock()
	defer a.historyMutex.Unlock()

	// 添加到历史记录
	a.predictionHistory = append(a.predictionHistory, record)

	// 如果记录太多，删除最旧的
	maxHistoryLength := 1000
	if len(a.predictionHistory) > maxHistoryLength {
		a.predictionHistory = a.predictionHistory[len(a.predictionHistory)-maxHistoryLength:]
	}
}

// checkPastPrediction 检查过去预测的准确性
func (a *MarketAnalyzer) checkPastPrediction(currentData *CoinData, now time.Time) {
	a.historyMutex.Lock()
	defer a.historyMutex.Unlock()

	// 检查10分钟前的预测
	targetTime := now.Add(-10 * time.Minute)

	// 找到最接近的记录
	var closestRecord *PredictionRecord
	minDiff := 5 * time.Minute

	for i := range a.predictionHistory {
		diff := targetTime.Sub(a.predictionHistory[i].Timestamp)
		if diff >= 0 && diff < minDiff {
			closestRecord = &a.predictionHistory[i]
			minDiff = diff
		}
	}

	// 如果找到了合适的记录
	if closestRecord != nil && minDiff < 2*time.Minute {
		// 计算价格变化
		priceChange := (currentData.Price - closestRecord.Price) / closestRecord.Price

		// 判断是否为平区间
		if closestRecord.Score >= 45 && closestRecord.Score <= 55 {
			timeStr := closestRecord.Timestamp.Format("2006-01-02 15:04:05")
			log.Printf("\n10分钟前预测验证 [%s]:", timeStr)
			log.Printf("预测分数: %.1f/100 (平区间，不计入胜率)\n", closestRecord.Score)
			log.Printf("实际价格变化: %.2f%%\n", priceChange*100)
			return
		}

		// 确定预测是否正确
		isCorrect := false
		if (closestRecord.Score > 55 && priceChange > 0) || (closestRecord.Score < 45 && priceChange < 0) {
			isCorrect = true
			a.winCount++
		}
		a.totalCount++

		// 使用if-else替代三元运算符
		expectedDirection := "下跌"
		if closestRecord.Score > 55 {
			expectedDirection = "上涨"
		}

		actualDirection := "下跌"
		if priceChange > 0 {
			actualDirection = "上涨"
		}

		predictionResult := "错误"
		if isCorrect {
			predictionResult = "正确"
		}

		// 输出验证结果
		timeStr := closestRecord.Timestamp.Format("2006-01-02 15:04:05")
		log.Printf("\n10分钟前预测验证 [%s]:", timeStr)
		log.Printf("预测分数: %.1f/100 (预期%s)\n", closestRecord.Score, expectedDirection)
		log.Printf("实际价格变化: %.2f%% (%s)\n", priceChange*100, actualDirection)
		log.Printf("预测结果: %s\n", predictionResult)
	}
}

// NewMarketAnalyzer 创建新的市场分析器
func NewMarketAnalyzer() *MarketAnalyzer {
	// 默认权重配置
	defaultWeights := map[string]float64{
		"price":       0.20, // 价格动量权重
		"flow":        0.25, // 资金流向权重
		"sentiment":   0.20, // 多空情绪权重
		"technical":   0.15, // 技术指标权重
		"liquidation": 0.10, // 清算压力权重
	}

	// 其他配置参数
	defaultConfig := map[string]interface{}{
		"priceChangeBounds": []float64{-0.3, 0.3},       // 价格变化范围
		"volChangeBounds":   []float64{-0.5, 0.5},       // 成交量变化范围
		"fundingBounds":     []float64{-0.0002, 0.0002}, // 资金费率范围
	}

	return &MarketAnalyzer{
		weights:           defaultWeights,
		config:            defaultConfig,
		predictionHistory: make([]PredictionRecord, 0, 100), // 初始容量为100的历史记录
		winCount:          0,
		totalCount:        0,
	}
}

// SetWeight 设置特定指标权重
func (a *MarketAnalyzer) SetWeight(indicator string, weight float64) {
	a.weights[indicator] = weight
	// 确保权重总和为1
	a.normalizeWeights()
}

// normalizeWeights 确保权重总和为1
func (a *MarketAnalyzer) normalizeWeights() {
	total := 0.0
	for _, weight := range a.weights {
		total += weight
	}

	if math.Abs(total-1.0) > 0.001 {
		for key, weight := range a.weights {
			a.weights[key] = weight / total
		}
	}
}

// FetchCoinData 从API获取币种数据
func (a *MarketAnalyzer) FetchCoinData() (*CoinData, error) {
	// 生成token
	apiKey := a.generateToken()

	// 创建请求
	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://api.coinank.com/api/instruments/agg?page=1&size=1", nil)
	if err != nil {
		return nil, err
	}

	// 设置请求头
	req.Header.Add("coinank-apikey", apiKey)

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 解析JSON
	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	// 检查是否有数据
	if !apiResp.Success || len(apiResp.Data.List) == 0 {
		return nil, fmt.Errorf("API请求失败或无数据")
	}

	return &apiResp.Data.List[0], nil
}

// generateToken 生成API请求token
func (a *MarketAnalyzer) generateToken() string {
	// 原始UUID
	e := "a2c903cc-b31e-c547-d299-b6d07b7631ab"

	// 截取前8位
	t := e[:8]

	// 移除前8位后拼接
	newE := e[8:] + t

	// 生成时间戳部分
	timeValue := time.Now().UnixNano() / int64(time.Millisecond)
	bigNumber := int64(1111111111111)
	n := fmt.Sprintf("%d347", timeValue+bigNumber)

	// 组合字符串
	combined := newE + "|" + n

	// Base64编码
	return toBase64(combined)
}

// toBase64 Base64编码实现
func toBase64(input string) string {
	base64Chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	result := ""
	i := 0

	for i < len(input) {
		char1 := int(input[i])
		i++
		char2 := 0
		if i < len(input) {
			char2 = int(input[i])
			i++
		}
		char3 := 0
		if i < len(input) {
			char3 = int(input[i])
			i++
		}

		byte1 := char1 >> 2
		byte2 := ((char1 & 3) << 4) | (char2 >> 4)
		byte3 := ((char2 & 15) << 2) | (char3 >> 6)
		byte4 := char3 & 63

		result += string(base64Chars[byte1])
		result += string(base64Chars[byte2])

		if char2 == 0 {
			result += "==" // 添加两个填充字符
			break          // 结束循环，已处理完所有输入
		} else if char3 == 0 {
			result += string(base64Chars[byte3])
			result += "=" // 添加一个填充字符
			break         // 结束循环，已处理完所有输入
		} else {
			result += string(base64Chars[byte3])
			result += string(base64Chars[byte4])
		}
	}

	return result
}

// CalculateTrendScore 计算趋势分数
func (a *MarketAnalyzer) CalculateTrendScore(data *CoinData) float64 {
	// 计算各个指标分数
	priceScore := a.calculatePriceScore(data)
	flowScore := a.calculateFlowScore(data)
	sentimentScore := a.calculateSentimentScore(data)
	technicalScore := a.calculateTechnicalScore(data)
	liquidationScore := a.calculateLiquidationScore(data)

	// 加权计算最终分数
	finalScore := priceScore*a.weights["price"] +
		flowScore*a.weights["flow"] +
		sentimentScore*a.weights["sentiment"] +
		technicalScore*a.weights["technical"] +
		liquidationScore*a.weights["liquidation"]

	// 确保分数在0-100范围内并四舍五入到小数点后1位
	return math.Round(math.Max(0, math.Min(finalScore, 100))*10) / 10
}

// calculatePriceScore 计算价格动量得分
func (a *MarketAnalyzer) calculatePriceScore(data *CoinData) float64 {
	// 价格变化权重
	priceWeights := []float64{0.3, 0.4, 0.3}
	priceChanges := []float64{data.PriceChangeM5, data.PriceChangeM15, data.PriceChangeM30}

	bounds := a.config["priceChangeBounds"].([]float64)
	lowerBound, upperBound := bounds[0], bounds[1]

	priceMomentum := 0.0
	for i, pc := range priceChanges {
		// 标准化：映射到0~100
		normalized := math.Max(0, math.Min(pc-lowerBound, upperBound-lowerBound)/(upperBound-lowerBound)*100)
		priceMomentum += normalized * priceWeights[i]
	}

	// 趋势一致性
	consistency := 1.0
	if (data.PriceChangeM5 > 0 && data.PriceChangeM15 > 0) || (data.PriceChangeM5 < 0 && data.PriceChangeM15 < 0) {
		consistency = 1.2 // 同向加分
	} else if (data.PriceChangeM5 > 0 && data.PriceChangeM15 < 0) || (data.PriceChangeM5 < 0 && data.PriceChangeM15 > 0) {
		consistency = 0.8 // 反向减分
	}

	return priceMomentum * consistency
}

// calculateFlowScore 计算资金流向得分
func (a *MarketAnalyzer) calculateFlowScore(data *CoinData) float64 {
	// 买卖力量：买盘占比
	buyPower := data.Buy5m / (data.Buy5m + data.Sell5m + 1e-6) * 100

	// 成交量变化率
	volumeChange := (data.Buy5m+data.Sell5m)/((data.Buy15m+data.Sell15m)/3+1e-6) - 1

	// 成交量得分标准化
	bounds := a.config["volChangeBounds"].([]float64)
	lowerBound, upperBound := bounds[0], bounds[1]
	volumeScore := math.Max(0, math.Min(volumeChange-lowerBound, upperBound-lowerBound)/(upperBound-lowerBound)*100)

	// 资金流向得分
	flowScore := buyPower*0.7 + volumeScore*0.3

	// 资金集中度：短期交易量占比
	shortTermRatio := (data.Buy5m + data.Sell5m) / (data.Buy24h + data.Sell24h + 1e-6) * 288
	concentration := math.Max(0, math.Min(shortTermRatio, 1)*100)

	return flowScore*0.7 + concentration*0.3
}

// calculateSentimentScore 计算多空情绪得分
func (a *MarketAnalyzer) calculateSentimentScore(data *CoinData) float64 {
	// 多空比评分，直接用longRatio和shortRatio
	// longRatio、shortRatio分别代表多头和空头人数占比，longShortRatio为多空资金比
	longRatio := data.LongShortRatio        // 资金多空比
	personLongRatio := data.LongShortRatio  // 兼容老字段，实际应为data.LongRatio
	personShortRatio := 1 - personLongRatio // 若API有longRatio/shortRatio字段可直接用

	// 多空比评分：资金多空比接近1为中性，>1偏多，<1偏空
	lsRatioScore := 100 - math.Abs(longRatio-1)/0.5*100
	lsRatioScore = math.Max(0, math.Min(lsRatioScore, 100))

	// 人数多空比评分（如有longRatio/shortRatio字段）
	personScore := 100 * (personLongRatio - personShortRatio + 1) / 2 // -1~1映射到0~100
	personScore = math.Max(0, math.Min(personScore, 100))

	// 资金费率评分
	bounds := a.config["fundingBounds"].([]float64)
	lowerBound, upperBound := bounds[0], bounds[1]
	fundingScore := math.Max(0, math.Min(data.FundingRate-lowerBound, upperBound-lowerBound)/(upperBound-lowerBound)*100)

	// 综合多空情绪得分
	return lsRatioScore*0.5 + personScore*0.2 + fundingScore*0.3
}

// calculateTechnicalScore 计算技术指标得分
func (a *MarketAnalyzer) calculateTechnicalScore(data *CoinData) float64 {
	// RSI信号
	rsiSignal := 0.0
	for _, rsi := range []float64{data.RsiM5, data.RsiM15} {
		if rsi >= 40 && rsi <= 60 {
			// 中性区间
			rsiSignal += 50 + (rsi-50)*2
		} else if rsi < 40 {
			// 超卖区间
			rsiSignal += 10 + (rsi-30)*4
		} else {
			// 超买区间
			rsiSignal += 90 - (rsi-60)*4
		}
	}
	rsiSignal /= 2 // 平均得分

	// 波动率趋势
	volTrend := data.VolatilityM5 / (data.VolatilityM15 + 1e-6)
	volScore := 0.0
	if volTrend > 1.2 {
		volScore = 70 + (math.Min(volTrend, 2)-1.2)/0.8*30
	} else {
		volScore = 70 - (1-volTrend/1.2)*70
	}

	return rsiSignal*0.6 + volScore*0.4
}

// calculateLiquidationScore 计算清算压力得分
func (a *MarketAnalyzer) calculateLiquidationScore(data *CoinData) float64 {
	// 直接用API返回的1小时清算数据
	liqLong := data.LiquidationH1Long
	liqShort := data.LiquidationH1Short
	liqTotal := data.LiquidationH1

	// 清算强度：总清算额标准化（如有更长周期可对比）
	liqIntensity := math.Max(0, math.Min(liqTotal/1e6, 1)*100) // 0~100，1e6为经验阈值

	// 多空清算比：空头清算/多头清算，>1说明多头更强
	liqBias := 50.0
	if liqLong+liqShort > 0 {
		liqBias = (liqShort / (liqLong + liqShort)) * 100 // 空头清算占比
	}

	// 综合清算压力得分
	return liqIntensity*0.4 + liqBias*0.6
}

// GetTrendSummary 获取趋势摘要信息
func (a *MarketAnalyzer) GetTrendSummary(data *CoinData) string {
	score := a.CalculateTrendScore(data)

	// 趋势评级
	var rating string
	switch {
	case score >= 75:
		rating = "强烈看涨"
	case score >= 60:
		rating = "看涨"
	case score >= 55:
		rating = "中性偏多"
	case score >= 45:
		rating = "中性"
	case score >= 40:
		rating = "中性偏空"
	case score >= 35:
		rating = "看跌"
	default:
		rating = "强烈看跌"
	}

	return fmt.Sprintf("趋势得分: %.1f/100 - %s", score, rating)
}

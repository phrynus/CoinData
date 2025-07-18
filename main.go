package main

import (
	"bytes"
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
	// 极端信号胜率统计 (score > 60 || score < 40)
	extremeWinCount   int
	extremeTotalCount int
}

var (
	dingWebhook = "https://oapi.dingtalk.com/robot/send?access_token=dc477bf40c7be545e31429d962c3c28946a6da114c7a9c75a4bea55f4d7a18a8"
)

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
	// executeCalculation(analyzer)
	c.AddFunc("2 * * * * *", func() {
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

	if score > 65 || score < 35 {
		err := SendDingTalkCustomMessage("BTC趋势预测", summary)
		if err != nil {
			log.Printf("发送钉钉消息失败: %v\n", err)
		}
	}

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
		log.Println("尚无足够历史数据计算胜率")
	}

	// 输出极端信号胜率 (score > 60 || score < 40)
	if analyzer.extremeTotalCount > 0 {
		extremeWinRate := float64(analyzer.extremeWinCount) / float64(analyzer.extremeTotalCount) * 100
		log.Printf("极端信号胜率: %.1f%% (正确: %d, 总计: %d)\n\n",
			extremeWinRate, analyzer.extremeWinCount, analyzer.extremeTotalCount)
	} else {
		log.Println("尚无足够极端信号数据计算胜率\n")
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

		// 确定预测是否正确
		isCorrect := false
		if (closestRecord.Score > 55 && priceChange > 0) || (closestRecord.Score < 45 && priceChange < 0) {
			isCorrect = true
			a.winCount++
		}
		a.totalCount++

		// 检查是否为极端信号 (score > 60 || score < 40)
		isExtreme := closestRecord.Score > 60 || closestRecord.Score < 40
		if isExtreme {
			if isCorrect {
				a.extremeWinCount++
			}
			a.extremeTotalCount++
		}

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
	// 优化后的权重配置 - 针对10分钟短期走势
	defaultWeights := map[string]float64{
		"price":       0.30, // 增加价格动量权重，短期内价格趋势更重要
		"flow":        0.30, // 增加资金流向权重，短期内买卖力量对价格影响明显
		"sentiment":   0.15, // 降低多空情绪权重，短期内波动快
		"technical":   0.20, // 增加技术指标权重，短期技术突破更有意义
		"liquidation": 0.05, // 降低清算压力权重，除非有大量清算
	}

	// 调整配置参数 - 针对更敏感的短期波动
	defaultConfig := map[string]interface{}{
		"priceChangeBounds":  []float64{-0.2, 0.2},       // 调窄价格变化范围，更敏感
		"volChangeBounds":    []float64{-0.7, 0.7},       // 扩大成交量变化范围，短期内波动大
		"fundingBounds":      []float64{-0.0002, 0.0002}, // 保持资金费率范围
		"shortTermIndicator": true,                       // 短期指标偏好
		"timeframe":          10,                         // 预测时间框架(分钟)
	}

	return &MarketAnalyzer{
		weights:           defaultWeights,
		config:            defaultConfig,
		predictionHistory: make([]PredictionRecord, 0, 100), // 初始容量为100的历史记录
		winCount:          0,
		totalCount:        0,
		extremeWinCount:   0,
		extremeTotalCount: 0,
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
	// 价格变化权重 - 调整为更看重近期变化
	priceWeights := []float64{0.6, 0.3, 0.1} // 更强调5分钟价格变化
	priceChanges := []float64{data.PriceChangeM5, data.PriceChangeM15, data.PriceChangeM30}

	bounds := a.config["priceChangeBounds"].([]float64)
	lowerBound, upperBound := bounds[0], bounds[1]

	priceMomentum := 0.0
	for i, pc := range priceChanges {
		// 标准化：映射到0~100
		normalized := math.Max(0, math.Min(pc-lowerBound, upperBound-lowerBound)/(upperBound-lowerBound)*100)
		priceMomentum += normalized * priceWeights[i]
	}

	// 趋势加速度 - 判断短期趋势是否加速
	acceleration := 0.0
	if math.Abs(data.PriceChangeM5) > math.Abs(data.PriceChangeM15)/3 {
		// 5分钟价格变化比15分钟平均变化更大，说明趋势正在加速
		if (data.PriceChangeM5 > 0 && data.PriceChangeM15 > 0) ||
			(data.PriceChangeM5 < 0 && data.PriceChangeM15 < 0) {
			// 同向加速
			acceleration = 15.0
		} else {
			// 反向加速（可能是反转）
			acceleration = -15.0
		}
	}

	// 趋势一致性
	consistency := 1.0
	if (data.PriceChangeM5 > 0 && data.PriceChangeM15 > 0) || (data.PriceChangeM5 < 0 && data.PriceChangeM15 < 0) {
		consistency = 1.2 // 同向加分
	} else if (data.PriceChangeM5 > 0 && data.PriceChangeM15 < 0) || (data.PriceChangeM5 < 0 && data.PriceChangeM15 > 0) {
		consistency = 0.8 // 反向减分
	}

	// 加入加速度因素
	return (priceMomentum * consistency) + acceleration
}

// calculateFlowScore 计算资金流向得分
func (a *MarketAnalyzer) calculateFlowScore(data *CoinData) float64 {
	// 买卖力量：买盘占比 - 标准化到40-60区间
	buyPower := 0.0
	if data.Buy5m+data.Sell5m > 0 {
		// 买盘占比转换到40-60区间，使其不会产生过于极端的值
		buyRatio := data.Buy5m / (data.Buy5m + data.Sell5m)
		buyPower = 40 + (buyRatio * 20) // 映射到40-60区间
	} else {
		buyPower = 50 // 默认中性
	}

	// 买卖力量变化趋势 - 使用更温和的计算方式
	var buyTrend float64 = 0

	if data.Buy15m+data.Sell15m > 0 && data.Buy5m+data.Sell5m > 0 {
		buyRatio5m := data.Buy5m / (data.Buy5m + data.Sell5m)
		buyRatio15m := data.Buy15m / (data.Buy15m + data.Sell15m)

		// 计算变化率并限制在合理范围内
		buyTrend = buyRatio5m - buyRatio15m

		// 缩放到适当范围 (-20到+20)，但避免过度放大
		if buyTrend > 0 {
			buyTrend = math.Min(buyTrend*100, 20)
		} else {
			buyTrend = math.Max(buyTrend*100, -20)
		}
	}

	// 成交量变化率 - 平滑处理极端值
	var volumeScore float64 = 50 // 默认中性值

	if data.Buy15m+data.Sell15m > 0 {
		// 5分钟成交量与15分钟平均值对比
		volumeRatio := (data.Buy5m + data.Sell5m) / ((data.Buy15m + data.Sell15m) / 3)

		// 对数变换，使极端值更平滑
		if volumeRatio > 0 {
			// 对数变换后缩放到0-100区间
			logVolChange := math.Log10(math.Max(volumeRatio, 0.1))
			// 将-1到1的对数值映射到25-75的分数区间
			volumeScore = 50 + logVolChange*25
			// 确保在有效范围内
			volumeScore = math.Max(25, math.Min(volumeScore, 75))
		}
	}

	// 成交量突变检测 - 更温和的影响
	volumeSpike := 0.0
	if data.Buy15m+data.Sell15m > 0 {
		volumeRatio := (data.Buy5m + data.Sell5m) / ((data.Buy15m + data.Sell15m) / 3)
		if volumeRatio > 2.0 {
			// 成交量突增，但限制最大影响
			volumeSpike = math.Min((volumeRatio-2.0)*5, 10)
		} else if volumeRatio < 0.5 {
			// 成交量骤减，但限制最大影响
			volumeSpike = math.Max((volumeRatio-0.5)*10, -10)
		}
	}

	// 资金流向得分 - 考虑买卖力量、趋势和成交量，每个因素的权重和影响更均衡
	flowScore := buyPower*0.5 + buyTrend*0.3 + volumeScore*0.2 + volumeSpike

	// 确保分数在0-100范围内
	return math.Max(0, math.Min(flowScore, 100))
}

// calculateSentimentScore 计算多空情绪得分
func (a *MarketAnalyzer) calculateSentimentScore(data *CoinData) float64 {
	// 多空比评分，直接用longRatio
	longRatio := data.LongShortRatio // 资金多空比
	// personLongRatio := data.LongShortRatio  // 兼容老字段，实际应为data.LongRatio

	// 多空比动态评分：基于当前市场状态的偏离度
	// 在震荡市场中，longRatio接近1最好；在趋势市场中，偏离度大可能更好
	lsRatioDynamicScore := 0.0

	// 根据价格变化趋势判断市场状态
	isTrendMarket := math.Abs(data.PriceChangeM15) > 0.3 || math.Abs(data.PriceChangeM5) > 0.15

	if isTrendMarket {
		// 趋势市场 - 多空比与价格变化同向为佳
		if (data.PriceChangeM5 > 0 && longRatio > 1.1) || (data.PriceChangeM5 < 0 && longRatio < 0.9) {
			lsRatioDynamicScore = 80 + (math.Min(math.Abs(longRatio-1), 0.5)/0.5)*20
		} else {
			// 多空比与价格变化方向不一致，可能是反转信号
			lsRatioDynamicScore = 40 - (math.Min(math.Abs(longRatio-1), 0.5)/0.5)*20
		}
	} else {
		// 震荡市场 - 多空比接近1为佳
		lsRatioDynamicScore = 100 - math.Abs(longRatio-1)/0.5*100
		lsRatioDynamicScore = math.Max(0, math.Min(lsRatioDynamicScore, 100))
	}

	// 资金费率评分 - 短期内资金费率变化不大，但仍有指示意义
	bounds := a.config["fundingBounds"].([]float64)
	lowerBound, upperBound := bounds[0], bounds[1]
	fundingScore := math.Max(0, math.Min(data.FundingRate-lowerBound, upperBound-lowerBound)/(upperBound-lowerBound)*100)

	// 综合多空情绪得分
	return lsRatioDynamicScore*0.8 + fundingScore*0.2
}

// calculateTechnicalScore 计算技术指标得分
func (a *MarketAnalyzer) calculateTechnicalScore(data *CoinData) float64 {
	// RSI信号 - 更关注5分钟RSI
	rsiM5Score := 0.0
	rsiM15Score := 0.0

	// 5分钟RSI信号 - 短期更敏感的阈值
	if data.RsiM5 <= 30 {
		// 严重超卖
		rsiM5Score = 80 + (30-data.RsiM5)*0.67 // 最高到100
	} else if data.RsiM5 <= 40 {
		// 轻度超卖
		rsiM5Score = 60 + (40-data.RsiM5)*2 // 60-80之间
	} else if data.RsiM5 >= 70 {
		// 严重超买
		rsiM5Score = 20 - (data.RsiM5-70)*0.67 // 最低到0
	} else if data.RsiM5 >= 60 {
		// 轻度超买
		rsiM5Score = 40 - (data.RsiM5-60)*2 // 20-40之间
	} else {
		// 中性区间 40-60
		rsiM5Score = 50 + (data.RsiM5-50)*0.5 // 45-55之间，接近50分
	}

	// 15分钟RSI信号 - 作为辅助确认
	if data.RsiM15 <= 30 {
		rsiM15Score = 80 + (30-data.RsiM15)*0.67
	} else if data.RsiM15 <= 40 {
		rsiM15Score = 60 + (40-data.RsiM15)*2
	} else if data.RsiM15 >= 70 {
		rsiM15Score = 20 - (data.RsiM15-70)*0.67
	} else if data.RsiM15 >= 60 {
		rsiM15Score = 40 - (data.RsiM15-60)*2
	} else {
		rsiM15Score = 50 + (data.RsiM15-50)*0.5
	}

	// RSI背离检测 - 价格与RSI方向不一致可能是重要信号
	rsiDivergence := 0.0
	if (data.PriceChangeM5 > 0 && data.RsiM5 < data.RsiM15) ||
		(data.PriceChangeM5 < 0 && data.RsiM5 > data.RsiM15) {
		rsiDivergence = -20.0 // RSI背离，看作反转信号
	}

	// 波动率趋势 - 波动率放大通常预示行情加速
	volRatio := data.VolatilityM5 / (data.VolatilityM15 + 1e-6)
	volScore := 0.0
	if volRatio > 1.3 {
		// 波动率显著放大，可能是突破或急跌
		volScore = 70 + (math.Min(volRatio, 2)-1.3)/0.7*30

		// 结合价格方向判断
		if math.Abs(data.PriceChangeM5) > 0.1 {
			volScore += 10 // 价格变化明显时波动率放大更有意义
		}
	} else if volRatio < 0.7 {
		// 波动率收缩，可能酝酿大行情
		volScore = 40 - (0.7-volRatio)/0.3*20
	} else {
		// 波动率平稳
		volScore = 40 + (volRatio-0.7)/0.6*30
	}

	// 计算最终技术分数 - 偏重5分钟RSI
	return rsiM5Score*0.6 + rsiM15Score*0.2 + volScore*0.2 + rsiDivergence
}

// calculateLiquidationScore 计算清算压力得分
func (a *MarketAnalyzer) calculateLiquidationScore(data *CoinData) float64 {
	// 直接用API返回的1小时清算数据
	liqLong := data.LiquidationH1Long
	liqShort := data.LiquidationH1Short
	liqTotal := data.LiquidationH1

	// 清算强度：总清算额标准化（针对短期影响调整）
	liqIntensity := math.Max(0, math.Min(liqTotal/5e5, 1)*100) // 阈值调整为50万，更敏感

	// 多空清算比例和方向
	liqDirection := 50.0
	if liqLong+liqShort > 1e4 { // 确保有足够清算量再计算方向
		// 空头清算占比高表示空头被迫平仓多，利好多头
		liqDirection = (liqShort / (liqLong + liqShort)) * 100
	}

	// 大量单向清算的额外影响
	liqBias := 0.0
	if liqTotal > 1e5 { // 大于10万的清算
		if liqShort > liqLong*3 {
			// 空头大量清算，可能引发短期拉升
			liqBias = 20.0
		} else if liqLong > liqShort*3 {
			// 多头大量清算，可能引发短期下跌
			liqBias = -20.0
		}
	}

	// 综合清算压力得分
	score := liqIntensity*0.3 + liqDirection*0.5 + liqBias
	return math.Max(0, math.Min(score, 100))
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

	return fmt.Sprintf("%.1f/100 - %s", score, rating)
}

// 发送钉钉自定义机器人消息 markdown
func SendDingTalkCustomMessage(title string, message string) error {
	type MarkdownMessage struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	}
	type DingTalkMessage struct {
		MsgType  string          `json:"msgtype"`
		Markdown MarkdownMessage `json:"markdown,omitempty"`
	}
	// Title取第一行
	// 构建钉钉消息
	msg := DingTalkMessage{
		MsgType: "markdown",
		Markdown: MarkdownMessage{
			Title: title,
			Text:  message,
		},
	}

	// 序列化消息
	jsonData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化钉钉消息失败: %v", err)
	}

	// 发送HTTP请求
	response, err := http.Post(dingWebhook, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("发送钉钉消息失败: %v", err)
	}
	defer response.Body.Close()

	// 检查响应状态
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("钉钉API返回错误状态码: %d", response.StatusCode)
	}
	// 检查响应内容
	var responseBody map[string]interface{}
	if err := json.NewDecoder(response.Body).Decode(&responseBody); err != nil {
		return fmt.Errorf("解析钉钉API响应失败: %v", err)
	}
	if responseBody["errcode"] != nil && responseBody["errcode"].(float64) != 0 {
		return fmt.Errorf("钉钉API返回错误: %v", responseBody["errmsg"])
	}

	return nil

}

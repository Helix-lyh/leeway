package deepseek

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// isPlatformAuthenticationError mirrors DeepSeekUsageFetcher
// .isPlatformAuthenticationError: the platform's authentication error codes.
func isPlatformAuthenticationError(code int) bool {
	return code == 40002 || code == 40003
}

// --- balance parsing ---------------------------------------------------------

// balance is the parsed wallet balance (mirrors DeepSeekUsageSnapshot's
// balance fields).
type balance struct {
	isAvailable     bool
	currency        string
	totalBalance    float64
	grantedBalance  float64
	toppedUpBalance float64
}

// balanceInfo mirrors DeepSeekBalanceInfo from the balance payload
// (is_available + balance_infos; amounts arrive as strings).
type balanceInfo struct {
	Currency        string `json:"currency"`
	TotalBalance    string `json:"total_balance"`
	GrantedBalance  string `json:"granted_balance"`
	ToppedUpBalance string `json:"topped_up_balance"`
}

// platformWallet mirrors DeepSeekPlatformWallet from the platform user
// summary payload; balance may be a number or a numeric string.
type platformWallet struct {
	Balance  float64
	Currency string
}

func (w *platformWallet) UnmarshalJSON(data []byte) error {
	var outer struct {
		Balance  json.RawMessage `json:"balance"`
		Currency string          `json:"currency"`
	}
	if err := json.Unmarshal(data, &outer); err != nil {
		return err
	}
	// Flexible scalar, mirroring the codex parser's flexFloat pattern: try a
	// string first (numbers may arrive quoted), then a plain number, and
	// reject when neither works (null/absent balance is an error).
	if len(outer.Balance) > 0 {
		var s string
		if err := json.Unmarshal(outer.Balance, &s); err == nil {
			f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
			if err != nil {
				return err
			}
			w.Balance = f
		} else {
			var f float64
			if err := json.Unmarshal(outer.Balance, &f); err != nil {
				return fmt.Errorf("invalid balance value %s", string(outer.Balance))
			}
			w.Balance = f
		}
	}
	w.Currency = outer.Currency
	return nil
}

// userSummaryResponse is the combined envelope for the balance endpoint.
// The platform returns {code, data:{biz_code, biz_data:{normal_wallets,
// bonus_wallets}}}; leeway's spec also accepts the balance payload shape
// {is_available, balance_infos} on the same endpoint (that is the shape the
// Swift fetcher parsed from api.deepseek.com/user/balance).
type userSummaryResponse struct {
	IsAvailable  *bool            `json:"is_available"`
	BalanceInfos []balanceInfo    `json:"balance_infos"`
	Code         *int             `json:"code"`
	Data         *userSummaryData `json:"data"`
}

type userSummaryData struct {
	BizCode *int         `json:"biz_code"`
	BizData *userBizData `json:"biz_data"`
}

type userBizData struct {
	NormalWallets []platformWallet `json:"normal_wallets"`
	BonusWallets  []platformWallet `json:"bonus_wallets"`
}

// parseBalance handles both response shapes of the balance endpoint,
// mirroring DeepSeekUsageFetcher.parseSnapshot (balance_infos) and
// .parsePlatformBalance (platform envelope).
func parseBalance(data []byte) (*balance, error) {
	var resp userSummaryResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, ParseFailed(err.Error())
	}

	// Platform envelope shape: {code, data:{biz_code, biz_data}}.
	if resp.Code != nil || resp.Data != nil {
		return parsePlatformBalance(resp)
	}

	// Balance payload shape: {is_available, balance_infos}.
	if resp.BalanceInfos != nil {
		return parseBalanceInfos(resp)
	}
	return nil, ParseFailed("Missing balance data in response")
}

// parsedBalanceInfo is a balance_infos row with numeric amounts.
type parsedBalanceInfo struct {
	currency string
	total    float64
	granted  float64
	toppedUp float64
}

// parseBalanceInfos mirrors DeepSeekUsageFetcher.parseSnapshot.
func parseBalanceInfos(resp userSummaryResponse) (*balance, error) {
	parsed := make([]parsedBalanceInfo, 0, len(resp.BalanceInfos))
	for _, info := range resp.BalanceInfos {
		total, err1 := strconv.ParseFloat(strings.TrimSpace(info.TotalBalance), 64)
		granted, err2 := strconv.ParseFloat(strings.TrimSpace(info.GrantedBalance), 64)
		toppedUp, err3 := strconv.ParseFloat(strings.TrimSpace(info.ToppedUpBalance), 64)
		if err1 != nil || err2 != nil || err3 != nil {
			return nil, ParseFailed("Non-numeric balance value in response.")
		}
		parsed = append(parsed, parsedBalanceInfo{
			currency: info.Currency,
			total:    total,
			granted:  granted,
			toppedUp: toppedUp,
		})
	}

	if len(parsed) == 0 {
		return &balance{currency: "USD"}, nil
	}

	// Prefer USD when it is funded, but do not hide a positive balance in
	// another currency behind an empty USD row (Swift selection order).
	selected := parsed[0]
	for _, b := range parsed {
		if b.currency == "USD" && b.total > 0 {
			selected = b
			break
		}
	}
	if !(selected.currency == "USD" && selected.total > 0) {
		for _, b := range parsed {
			if b.total > 0 {
				selected = b
				break
			}
		}
	}
	if !(selected.total > 0) {
		for _, b := range parsed {
			if b.currency == "USD" {
				selected = b
				break
			}
		}
	}

	isAvailable := resp.IsAvailable != nil && *resp.IsAvailable
	return &balance{
		isAvailable:     isAvailable,
		currency:        selected.currency,
		totalBalance:    selected.total,
		grantedBalance:  selected.granted,
		toppedUpBalance: selected.toppedUp,
	}, nil
}

// parsePlatformBalance mirrors DeepSeekUsageFetcher.parsePlatformBalance.
func parsePlatformBalance(resp userSummaryResponse) (*balance, error) {
	if resp.Code != nil && *resp.Code != 0 {
		if isPlatformAuthenticationError(*resp.Code) {
			return nil, InvalidPlatformToken()
		}
		return nil, APIError(fmt.Sprintf("user summary code %d", *resp.Code))
	}
	if resp.Data != nil && resp.Data.BizCode != nil && *resp.Data.BizCode != 0 {
		if isPlatformAuthenticationError(*resp.Data.BizCode) {
			return nil, InvalidPlatformToken()
		}
		return nil, APIError(fmt.Sprintf("user summary biz_code %d", *resp.Data.BizCode))
	}
	if resp.Data == nil || resp.Data.BizData == nil {
		return nil, ParseFailed("Missing user summary biz_data")
	}
	biz := resp.Data.BizData

	toppedUp := map[string]float64{}
	granted := map[string]float64{}
	for _, w := range biz.NormalWallets {
		toppedUp[w.Currency] += w.Balance
	}
	for _, w := range biz.BonusWallets {
		granted[w.Currency] += w.Balance
	}
	currencySet := map[string]bool{}
	for c := range toppedUp {
		currencySet[c] = true
	}
	for c := range granted {
		currencySet[c] = true
	}
	currencies := make([]string, 0, len(currencySet))
	for c := range currencySet {
		currencies = append(currencies, c)
	}
	sort.Strings(currencies)
	if len(currencies) == 0 {
		return &balance{currency: "USD"}, nil
	}

	selected := ""
	for _, c := range currencies {
		if c == "USD" && toppedUp[c]+granted[c] > 0 {
			selected = c
			break
		}
	}
	if selected == "" {
		for _, c := range currencies {
			if toppedUp[c]+granted[c] > 0 {
				selected = c
				break
			}
		}
	}
	if selected == "" {
		for _, c := range currencies {
			if c == "USD" {
				selected = c
				break
			}
		}
	}
	if selected == "" {
		selected = currencies[0]
	}

	total := toppedUp[selected] + granted[selected]
	return &balance{
		isAvailable:     total > 0,
		currency:        selected,
		totalBalance:    total,
		grantedBalance:  granted[selected],
		toppedUpBalance: toppedUp[selected],
	}, nil
}

// --- usage parsing (amount + cost) -------------------------------------------

// usageSummary mirrors DeepSeekUsageSummary, restricted to the fields leeway
// surfaces: per-day usage windows plus the aggregates used for tests.
type usageSummary struct {
	todayTokens              int64
	currentMonthTokens       int64
	todayCost                *float64
	currentMonthCost         *float64
	currentMonthRequestCount int64
	topModel                 string
	categoryBreakdown        []categoryBreakdown
	daily                    []dailyUsage
	currency                 string
}

type categoryBreakdown struct {
	category string // canonical type: PROMPT_CACHE_HIT_TOKEN etc.
	tokens   int64
	cost     *float64
}

type dailyUsage struct {
	date         string
	totalTokens  int64
	cost         *float64
	requestCount int64
}

// usageItem mirrors DeepSeekUsageItem / DeepSeekCostItem (both are
// {type, amount} pairs).
type usageItem struct {
	Type   string `json:"type"`
	Amount string `json:"amount"`
}

type modelUsage struct {
	Model string      `json:"model"`
	Usage []usageItem `json:"usage"`
}

type dayUsage struct {
	Date string       `json:"date"`
	Data []modelUsage `json:"data"`
}

// amountBizData mirrors DeepSeekAmountBizData.
type amountBizData struct {
	Total []modelUsage `json:"total"`
	Days  []dayUsage   `json:"days"`
}

// costBizDataItem mirrors DeepSeekCostBizDataItem. Note biz_data in the cost
// payload is an ARRAY of these items; only the first is used.
type costBizDataItem struct {
	Total    []modelUsage `json:"total"`
	Days     []dayUsage   `json:"days"`
	Currency *string      `json:"currency"`
}

// amountPayload mirrors DeepSeekAmountPayload. Error envelopes are not
// schema-stable: the code is preserved even when `data` has an unexpected
// shape (decoding of data is only attempted on the success path).
type amountPayload struct {
	Code    *int
	Data    *amountPayloadData
	BizData *amountBizData
}

type amountPayloadData struct {
	BizCode *int
}

// decodeEnvelope decodes the {code, data:{biz_code, biz_data}} envelope shared
// by the amount and cost payloads. Error envelopes are not schema-stable: the
// code is preserved even when `data` has an unexpected shape (biz_data is
// decoded only on the success path). bizUnmarshal decodes the payload's
// concrete biz_data type.
func decodeEnvelope[T any](data []byte, bizUnmarshal func(json.RawMessage) (*T, error)) (code *int, bizCode *int, biz *T, err error) {
	var outer struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &outer); err != nil {
		return nil, nil, nil, err
	}
	code = outer.Code
	if code == nil || *code == 0 {
		if len(outer.Data) > 0 && string(outer.Data) != "null" {
			var inner struct {
				BizCode *int            `json:"biz_code"`
				BizData json.RawMessage `json:"biz_data"`
			}
			if err := json.Unmarshal(outer.Data, &inner); err != nil {
				return nil, nil, nil, err
			}
			bizCode = inner.BizCode
			if inner.BizCode == nil || *inner.BizCode == 0 {
				if len(inner.BizData) > 0 && string(inner.BizData) != "null" {
					if biz, err = bizUnmarshal(inner.BizData); err != nil {
						return nil, nil, nil, err
					}
				}
			}
		}
	}
	return code, bizCode, biz, nil
}

func decodeAmountPayload(data []byte) (*amountPayload, error) {
	code, bizCode, biz, err := decodeEnvelope(data, func(raw json.RawMessage) (*amountBizData, error) {
		var b amountBizData
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, err
		}
		return &b, nil
	})
	if err != nil {
		return nil, err
	}
	payload := &amountPayload{Code: code}
	if code == nil || *code == 0 {
		payload.Data = &amountPayloadData{BizCode: bizCode}
		if biz != nil {
			payload.BizData = biz
		}
	}
	return payload, nil
}

// costPayload mirrors DeepSeekCostPayload (biz_data is an array).
type costPayload struct {
	Code    *int
	Data    *costPayloadData
	BizData []costBizDataItem
}

type costPayloadData struct {
	BizCode *int
}

func decodeCostPayload(data []byte) (*costPayload, error) {
	code, bizCode, biz, err := decodeEnvelope(data, func(raw json.RawMessage) (*[]costBizDataItem, error) {
		var b []costBizDataItem
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, err
		}
		return &b, nil
	})
	if err != nil {
		return nil, err
	}
	payload := &costPayload{Code: code}
	if code == nil || *code == 0 {
		payload.Data = &costPayloadData{BizCode: bizCode}
		if biz != nil {
			payload.BizData = *biz
		}
	}
	return payload, nil
}

// categoryOf mirrors DeepSeekUsageCategory(rawValue:): the type string is
// matched case-insensitively against the four known categories.
func categoryOf(typeStr string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(typeStr)) {
	case "PROMPT_CACHE_HIT_TOKEN":
		return "PROMPT_CACHE_HIT_TOKEN", true
	case "PROMPT_CACHE_MISS_TOKEN":
		return "PROMPT_CACHE_MISS_TOKEN", true
	case "RESPONSE_TOKEN":
		return "RESPONSE_TOKEN", true
	case "REQUEST":
		return "REQUEST", true
	}
	return "", false
}

// parseTokenAmount mirrors DeepSeekUsageCostParser.parseTokenAmount.
func parseTokenAmount(value string) int64 {
	if n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
		return n
	}
	return 0
}

// parseCostAmount mirrors DeepSeekUsageCostParser.parseCostAmount.
func parseCostAmount(value string) float64 {
	if f, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
		return f
	}
	return 0
}

// parseUsageSummary mirrors DeepSeekUsageCostParser.parse: decode both
// payloads, validate envelope codes, then aggregate.
func parseUsageSummary(amountData, costData []byte, now time.Time) (*usageSummary, error) {
	amountPayload, err := decodeAmountPayload(amountData)
	if err != nil {
		return nil, ParseFailed("amount: " + err.Error())
	}
	costPayload, err := decodeCostPayload(costData)
	if err != nil {
		return nil, ParseFailed("cost: " + err.Error())
	}

	if amountPayload.Code != nil && *amountPayload.Code != 0 {
		if isPlatformAuthenticationError(*amountPayload.Code) {
			return nil, InvalidPlatformToken()
		}
		return nil, APIError(fmt.Sprintf("amount code %d", *amountPayload.Code))
	}
	if amountPayload.Data != nil && amountPayload.Data.BizCode != nil && *amountPayload.Data.BizCode != 0 {
		if isPlatformAuthenticationError(*amountPayload.Data.BizCode) {
			return nil, InvalidPlatformToken()
		}
		return nil, APIError(fmt.Sprintf("amount biz_code %d", *amountPayload.Data.BizCode))
	}
	if costPayload.Code != nil && *costPayload.Code != 0 {
		if isPlatformAuthenticationError(*costPayload.Code) {
			return nil, InvalidPlatformToken()
		}
		return nil, APIError(fmt.Sprintf("cost code %d", *costPayload.Code))
	}
	if costPayload.Data != nil && costPayload.Data.BizCode != nil && *costPayload.Data.BizCode != 0 {
		if isPlatformAuthenticationError(*costPayload.Data.BizCode) {
			return nil, InvalidPlatformToken()
		}
		return nil, APIError(fmt.Sprintf("cost biz_code %d", *costPayload.Data.BizCode))
	}
	if amountPayload.BizData == nil {
		return nil, ParseFailed("Missing amount biz_data")
	}

	currency := "CNY"
	if len(costPayload.BizData) > 0 && costPayload.BizData[0].Currency != nil {
		currency = *costPayload.BizData[0].Currency
	}
	return aggregateUsage(amountPayload.BizData, firstCostItem(costPayload.BizData), currency, now), nil
}

func firstCostItem(items []costBizDataItem) *costBizDataItem {
	if len(items) == 0 {
		return nil
	}
	return &items[0]
}

// usageSummary =================================================================

// aggregateUsage mirrors DeepSeekUsageCostParser.aggregate.
func aggregateUsage(amountBiz *amountBizData, costBiz *costBizDataItem, currency string, now time.Time) *usageSummary {
	ctx := newAggregationContext(amountBiz.Days, costDays(costBiz), now)
	today := aggregateDay(ctx.todayString, ctx.amountMap, ctx.costMap)
	month := aggregateMonth(ctx)
	topModel, breakdown := buildBreakdowns(amountBiz.Total, costTotals(costBiz))
	return &usageSummary{
		todayTokens:              today.tokens,
		currentMonthTokens:       month.tokens,
		todayCost:                today.cost,
		currentMonthCost:         month.cost,
		currentMonthRequestCount: month.requests,
		topModel:                 topModel,
		categoryBreakdown:        breakdown,
		daily:                    buildDailyUsages(ctx),
		currency:                 currency,
	}
}

func costDays(costBiz *costBizDataItem) []dayUsage {
	if costBiz == nil {
		return nil
	}
	return costBiz.Days
}

func costTotals(costBiz *costBizDataItem) []modelUsage {
	if costBiz == nil {
		return nil
	}
	return costBiz.Total
}

// dayAggregationResult mirrors DeepSeekUsageCostParser.DayAggregationResult.
type dayAggregationResult struct {
	tokens   int64
	cost     *float64
	requests int64
}

// aggregationContext mirrors DeepSeekUsageCostParser.AggregationContext.
type aggregationContext struct {
	todayString  string
	startOfMonth time.Time
	now          time.Time
	amountMap    map[string]map[string][]usageItem
	costMap      map[string]map[string][]usageItem
	allDates     []string // sorted
}

func newAggregationContext(amountDays, costDays []dayUsage, now time.Time) *aggregationContext {
	ctx := &aggregationContext{
		todayString: now.UTC().Format("2006-01-02"),
		now:         now.UTC(),
		amountMap:   buildUsageMap(amountDays),
		costMap:     buildUsageMap(costDays),
	}
	firstOfMonth := time.Date(ctx.now.Year(), ctx.now.Month(), 1, 0, 0, 0, 0, time.UTC)
	ctx.startOfMonth = firstOfMonth

	dateSet := map[string]bool{}
	for date := range ctx.amountMap {
		dateSet[date] = true
	}
	for date := range ctx.costMap {
		dateSet[date] = true
	}
	for date := range dateSet {
		ctx.allDates = append(ctx.allDates, date)
	}
	sort.Strings(ctx.allDates)
	return ctx
}

// buildUsageMap mirrors DeepSeekUsageCostParser.AggregationContext
// .buildAmountMap/.buildCostMap: date -> model -> non-empty usage items.
func buildUsageMap(days []dayUsage) map[string]map[string][]usageItem {
	result := map[string]map[string][]usageItem{}
	for _, day := range days {
		if day.Date == "" {
			continue
		}
		modelMap := map[string][]usageItem{}
		for _, modelUsage := range day.Data {
			if modelUsage.Model == "" {
				continue
			}
			if len(modelUsage.Usage) > 0 {
				modelMap[modelUsage.Model] = modelUsage.Usage
			}
		}
		if len(modelMap) > 0 {
			result[day.Date] = modelMap
		}
	}
	return result
}

// sumDay aggregates one day's tokens, cost and request count,
// mirroring the innermost loops of DeepSeekUsageCostParser.aggregateDay.
func sumDay(amountItems, costItems []usageItem) dayAggregationResult {
	var out dayAggregationResult
	for _, item := range amountItems {
		category, ok := categoryOf(item.Type)
		if !ok {
			continue
		}
		if category == "REQUEST" {
			out.requests += parseTokenAmount(item.Amount)
		} else {
			out.tokens += parseTokenAmount(item.Amount)
		}
	}
	var costTotal float64
	hasCost := false
	for _, item := range costItems {
		category, ok := categoryOf(item.Type)
		if !ok {
			continue
		}
		if category != "REQUEST" {
			costTotal += parseCostAmount(item.Amount)
			hasCost = true
		}
	}
	if hasCost {
		cost := costTotal
		out.cost = &cost
	}
	return out
}

// aggregateDay mirrors DeepSeekUsageCostParser.aggregateDay.
func aggregateDay(dateString string, amountMap, costMap map[string]map[string][]usageItem) dayAggregationResult {
	var amountItems, costItems []usageItem
	for _, items := range amountMap[dateString] {
		amountItems = append(amountItems, items...)
	}
	for _, items := range costMap[dateString] {
		costItems = append(costItems, items...)
	}
	return sumDay(amountItems, costItems)
}

// aggregateMonth mirrors DeepSeekUsageCostParser.aggregateMonth: sums every
// day whose date falls in [startOfMonth, now].
func aggregateMonth(ctx *aggregationContext) dayAggregationResult {
	var out dayAggregationResult
	for _, date := range ctx.allDates {
		parsed, ok := parseDayDate(date)
		if !ok || parsed.Before(ctx.startOfMonth) || parsed.After(ctx.now) {
			continue
		}
		res := aggregateDay(date, ctx.amountMap, ctx.costMap)
		out.tokens += res.tokens
		out.requests += res.requests
		if res.cost != nil {
			if out.cost == nil {
				cost := *res.cost
				out.cost = &cost
			} else {
				*out.cost += *res.cost
			}
		}
	}
	return out
}

// buildBreakdowns mirrors DeepSeekUsageCostParser.buildBreakdowns: top model
// by total tokens (ties resolved to the lexicographically smaller model, as
// Swift's max(by:) does) and the three token-category breakdown rows.
func buildBreakdowns(totalAmounts, totalCosts []modelUsage) (string, []categoryBreakdown) {
	modelTokens := map[string]int64{}
	categoryTokens := map[string]int64{}
	categoryCosts := map[string]float64{}

	for _, modelUsage := range totalAmounts {
		if modelUsage.Model == "" {
			continue
		}
		var total int64
		for _, item := range modelUsage.Usage {
			category, ok := categoryOf(item.Type)
			if !ok || category == "REQUEST" {
				continue
			}
			amount := parseTokenAmount(item.Amount)
			total += amount
			categoryTokens[category] += amount
		}
		modelTokens[modelUsage.Model] = total
	}
	for _, costUsage := range totalCosts {
		if costUsage.Model == "" {
			continue
		}
		for _, item := range costUsage.Usage {
			category, ok := categoryOf(item.Type)
			if !ok || category == "REQUEST" {
				continue
			}
			categoryCosts[category] += parseCostAmount(item.Amount)
		}
	}

	topModel := ""
	best := int64(-1)
	for model, total := range modelTokens {
		if total > best || (total == best && (topModel == "" || model < topModel)) {
			topModel = model
			best = total
		}
	}

	// Fixed order mirrors Swift's breakdown array.
	order := []string{"PROMPT_CACHE_HIT_TOKEN", "PROMPT_CACHE_MISS_TOKEN", "RESPONSE_TOKEN"}
	breakdown := make([]categoryBreakdown, 0, len(order))
	for _, category := range order {
		var cost *float64
		if c, ok := categoryCosts[category]; ok {
			c := c
			cost = &c
		}
		breakdown = append(breakdown, categoryBreakdown{
			category: category,
			tokens:   categoryTokens[category],
			cost:     cost,
		})
	}
	return topModel, breakdown
}

// buildDailyUsages mirrors DeepSeekUsageCostParser.buildDailyUsages: one row
// per day within the current month, sorted by date.
func buildDailyUsages(ctx *aggregationContext) []dailyUsage {
	var out []dailyUsage
	for _, date := range ctx.allDates {
		parsed, ok := parseDayDate(date)
		if !ok || parsed.Before(ctx.startOfMonth) || parsed.After(ctx.now) {
			continue
		}
		res := aggregateDay(date, ctx.amountMap, ctx.costMap)
		out = append(out, dailyUsage{
			date:         date,
			totalTokens:  res.tokens,
			cost:         res.cost,
			requestCount: res.requests,
		})
	}
	return out
}

// parseDayDate mirrors DeepSeekUsageCostParser.parseDate (yyyy-MM-dd, UTC).
func parseDayDate(text string) (time.Time, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

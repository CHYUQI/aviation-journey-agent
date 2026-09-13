package data

import "aviation-journey-agent/backend/internal/textutil"

// VerifyQuote 校验模型给出的原文摘录确实来自抓取到的页面。
//
// 这是拦住"模型编数字"的机械手段：解析网页时要求模型在返回结构化字段的同时，
// 附上一段它依据的原文摘录，代码再回到页面文本里查找这段摘录。
//
//	找得到 → 采信，按来源可信度给 confidence
//	找不到 → 丢弃这条观测值，宁可字段留 null
//
// 具体实现见 textutil：归一化会去掉 HTML 标签、解开实体、折叠空白，
// 避免页面里的换行、缩进和 &nbsp; 导致误判。
func VerifyQuote(pageText, quote string) bool {
	return textutil.ContainsQuote(pageText, quote)
}

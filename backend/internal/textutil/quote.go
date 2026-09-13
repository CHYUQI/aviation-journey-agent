// Package textutil 提供文本归一化与"原文引用校验"。
//
// 用途：模型从网页文本里抽字段时，我们要求它同时交出依据的原文片段，
// 再回到页面文本里查找。找得到才采信，找不到就丢弃。
// 这是拦住"模型编数字"的机械手段 —— 不依赖模型自觉。
package textutil

import (
	"html"
	"regexp"
	"strings"
)

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// Normalize 把文本归一化，消除"同一句话在不同排版下写法不同"带来的误判：
// 去掉 HTML 标签、解开 HTML 实体、折叠连续空白、统一全角空格。
//
// 先去标签再解实体，顺序不能反：先解实体会把页面里原本显示为文字的
// &lt;div&gt; 变成真标签而被误删。
func Normalize(s string) string {
	s = htmlTagPattern.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\u3000", " ")
	return strings.Join(strings.Fields(s), " ")
}

// Contains 判断 needle 是否出现在 text 里（两边都归一化后比较）。
//
// 用于校验结构化取值，例如登机口 "B64"、时间 "08:00"。
func Contains(text, needle string) bool {
	needle = Normalize(needle)
	if needle == "" {
		return false
	}
	return strings.Contains(Normalize(text), needle)
}

// minQuoteRunes 是自由文本引用摘录的最短长度。
//
// 太短的摘录（比如只有 "47"）几乎能在任何网页里匹配到，起不到验证作用。
// 结构化取值请用 Contains，不受这个限制。
const minQuoteRunes = 6

// ContainsQuote 判断一段自由文本摘录是否真的来自页面。
func ContainsQuote(pageText, quote string) bool {
	normalized := Normalize(quote)
	if len([]rune(normalized)) < minQuoteRunes {
		return false
	}
	return strings.Contains(Normalize(pageText), normalized)
}

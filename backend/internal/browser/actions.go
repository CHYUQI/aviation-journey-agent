package browser

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ClickText 点击页面上文本等于 text 的元素。
//
// 逐个点它自己和往上四层父元素，因为可点区域常常是包着文字的容器。
// 用文本定位而不是 CSS 选择器：航司页面改版时类名天天变，
// 但按钮上的字一般比较稳定。
func (b *Browser) ClickText(text string) error {
	expr := fmt.Sprintf(`(() => {
		const target = [...document.querySelectorAll('*')].find(
			e => e.children.length === 0 && (e.innerText || '').trim() === %s)
		if (!target) return 'not-found'
		let node = target
		for (let i = 0; i < 4 && node; i++) { node.click(); node = node.parentElement }
		return 'ok'
	})()`, jsLiteral(text))

	return b.expectOK(expr, fmt.Sprintf("找不到文本为 %q 的可点击元素", text))
}

// ClickSelector 按 CSS 选择器点击元素。
//
// 有些提交按钮是 <input type="button"> 且没有文字，按文字点不到，
// 只能退回选择器。选择器比文字脆弱，所以只在必要时用。
func (b *Browser) ClickSelector(selector string) error {
	expr := fmt.Sprintf(`(() => {
		const el = document.querySelector(%s)
		if (!el) return 'not-found'
		el.click()
		return 'ok'
	})()`, jsLiteral(selector))

	return b.expectOK(expr, fmt.Sprintf("找不到选择器为 %q 的元素", selector))
}

// Fill 往输入框里填值。
//
// 必须用原生 setter 再派发 input 事件：React/Vue 会劫持 value 属性，
// 直接赋值不会触发框架的状态更新，表单看起来填了但提交时是空的。
func (b *Browser) Fill(selector, value string) error {
	expr := fmt.Sprintf(`(() => {
		const el = document.querySelector(%s)
		if (!el) return 'not-found'
		const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
		setter.call(el, %s)
		el.dispatchEvent(new Event('input', { bubbles: true }))
		el.dispatchEvent(new Event('change', { bubbles: true }))
		return 'ok'
	})()`, jsLiteral(selector), jsLiteral(value))

	return b.expectOK(expr, fmt.Sprintf("找不到选择器为 %q 的输入框", selector))
}

// WaitText 轮询页面文本，直到匹配上正则或超时。
func (b *Browser) WaitText(pattern string, timeout time.Duration) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("等待条件不是合法的正则: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		text, err := b.PageText()
		if err == nil && re.MatchString(text) {
			return nil
		}
		time.Sleep(700 * time.Millisecond)
	}
	return fmt.Errorf("等待 %q 超时（%s）", pattern, timeout)
}

// WaitSelector 轮询等待某个元素出现在页面上。
//
// 比等文本更精确：输入框的提示文字是 placeholder，不属于可见文本，
// 等文本永远等不到。
func (b *Browser) WaitSelector(selector string, timeout time.Duration) error {
	expr := fmt.Sprintf("document.querySelector(%s) ? 'ok' : 'not-found'", jsLiteral(selector))

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		result, err := b.EvaluateString(expr)
		if err == nil && strings.TrimSpace(result) == "ok" {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("等待元素 %q 超时（%s）", selector, timeout)
}

// ElementText 返回某个元素的可见文本，找不到时返回空字符串。
// 调试配方时用它看"点完之后到底出现了什么"。
func (b *Browser) ElementText(selector string) (string, error) {
	return b.EvaluateString(fmt.Sprintf(
		"(document.querySelector(%s) || {}).innerText || ''", jsLiteral(selector)))
}

// Sleep 等待一段时间。SPA 挂载完成前点击会点空，所以需要显式等待。
func (b *Browser) Sleep(d time.Duration) {
	time.Sleep(d)
}

func (b *Browser) expectOK(expr, errMsg string) error {
	result, err := b.EvaluateString(expr)
	if err != nil {
		return err
	}
	if strings.TrimSpace(result) != "ok" {
		return fmt.Errorf("%s", errMsg)
	}
	return nil
}

// jsLiteral 把字符串转成安全的 JS 字面量（会做 JSON 转义，防注入）。
func jsLiteral(s string) string {
	raw, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(raw)
}

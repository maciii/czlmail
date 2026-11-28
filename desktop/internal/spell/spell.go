// Package spell 用操作系统自带的拼写检查器同时按多种语言检查拼写。
//
// WebView2 自带的检查只认一种语言 (见 package main 的 spellcheck.go)。用两种语言写信的人 (常常一封信里就有两种),
// 需要一个词在任一语言里正确就不标, 还要能把一段文字标成某一种语言。Windows 拼写检查 API
// 用 Windows 已有的词典 (含用户自己加的词) 就能做到。
//
// 位置一律是 UTF-16 码元, 与 JavaScript 字符串下标一致, 编辑器可以直接用。
package spell

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Action 是拼写检查器对一处错误给出的处理方式 (Windows 里的 CORRECTIVE_ACTION)。
type Action int

const (
	ActionNone Action = iota
	// ActionSuggest: 词不认识, 向 Suggest 要改法。
	ActionSuggest
	// ActionReplace: 用 Replacement 替换这个词 (常见拼错, 只有一种改法)。
	ActionReplace
	// ActionDelete: 删掉这段文字 (重复的词)。
	ActionDelete
)

// Error 是被检查文字里的一处拼写错误范围。
type Error struct {
	// Start 与 Length 以 UTF-16 码元计。
	Start       int    `json:"start"`
	Length      int    `json:"length"`
	Action      Action `json:"action"`
	Replacement string `json:"replacement,omitempty"`
}

// Merge 只保留每种语言都报的错误: 一个词只有在没有任何语言认可时才算错。
// perLang 是同一段文字在各语言下的错误。
func Merge(perLang [][]Error) []Error {
	if len(perLang) == 0 {
		return nil
	}
	type key struct{ start, length int }
	count := map[key]int{}
	for _, errs := range perLang {
		seen := map[key]bool{}
		for _, e := range errs {
			k := key{e.Start, e.Length}
			if !seen[k] {
				seen[k] = true
				count[k]++
			}
		}
	}
	var out []Error
	for _, e := range perLang[0] {
		if count[key{e.Start, e.Length}] == len(perLang) {
			out = append(out, e)
			count[key{e.Start, e.Length}] = 0 // 每个范围只报一次
		}
	}
	return out
}

// MergeSuggestions 把各语言的建议交错合并, 最好的在前, 不重复。
func MergeSuggestions(perLang [][]string, limit int) []string {
	seen := map[string]bool{}
	var out []string
	for i := 0; len(out) < limit; i++ {
		more := false
		for _, list := range perLang {
			if i >= len(list) {
				continue
			}
			more = true
			s := list[i]
			if k := strings.ToLower(s); !seen[k] {
				seen[k] = true
				out = append(out, s)
				if len(out) == limit {
					break
				}
			}
		}
		if !more {
			break
		}
	}
	return out
}

// ByCloseness 给各语言的建议列表排序, 最好的几条离原词最近的语言排在前面, 每种语言内部的顺序不变。
// 这样英文拼错 ("speling") 时, 排前面的是英文列表 ("spelling"), 而不是只是长得像的捷克语词 ("spalin")。
func ByCloseness(word string, perLang [][]string) [][]string {
	best := make([]int, len(perLang))
	for i, list := range perLang {
		best[i] = -1
		for j, s := range list {
			if j == 3 {
				break
			}
			if d := distance(word, s); best[i] < 0 || d < best[i] {
				best[i] = d
			}
		}
	}
	idx := make([]int, len(perLang))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		da, db := best[idx[a]], best[idx[b]]
		if da < 0 || db < 0 {
			return db < 0 && da >= 0
		}
		return da < db
	})
	out := make([][]string, len(perLang))
	for i, k := range idx {
		out[i] = perLang[k]
	}
	return out
}

// distance 是以半步为单位的编辑距离: 只改大小写或变音符号算 1, 其他插入、删除、替换算 2。
// "kun" 离 "kůň" 比离 "kino" 近。
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = 2 * j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = 2 * i
		for j := 1; j <= len(rb); j++ {
			sub := prev[j-1]
			if ra[i-1] != rb[j-1] {
				if base(ra[i-1]) == base(rb[j-1]) {
					sub++
				} else {
					sub += 2
				}
			}
			cur[j] = min(sub, prev[j]+2, cur[j-1]+2)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// base 是去掉大小写与变音符号的字母。
func base(r rune) rune {
	for _, c := range norm.NFD.String(string(unicode.ToLower(r))) {
		return c
	}
	return r
}

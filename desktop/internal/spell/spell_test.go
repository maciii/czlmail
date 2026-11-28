package spell

import (
	"reflect"
	"testing"
)

func TestMergeKeepsWordsNoLanguageAccepts(t *testing.T) {
	cs := []Error{{Start: 0, Length: 6, Action: ActionSuggest}, {Start: 10, Length: 7, Action: ActionSuggest}}
	en := []Error{{Start: 10, Length: 7, Action: ActionSuggest}, {Start: 20, Length: 4, Action: ActionSuggest}}
	got := Merge([][]Error{cs, en})
	want := []Error{{Start: 10, Length: 7, Action: ActionSuggest}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Merge = %v, want %v", got, want)
	}
	if got := Merge([][]Error{cs}); !reflect.DeepEqual(got, cs) {
		t.Errorf("one language = %v", got)
	}
	if got := Merge(nil); got != nil {
		t.Errorf("no language = %v", got)
	}
}

func TestMergeSuggestionsInterleaves(t *testing.T) {
	got := MergeSuggestions([][]string{{"kůň", "kun", "kuna"}, {"Kun", "kin"}}, 4)
	want := []string{"kůň", "Kun", "kin", "kuna"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MergeSuggestions = %v, want %v", got, want)
	}
}

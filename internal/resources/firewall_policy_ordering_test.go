package resources

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCompleteOrdering(t *testing.T) {
	for name, tc := range map[string]struct {
		want, current, expected policyOrdering
	}{
		"policy removed from config is kept on its side until deleted": {
			want:     policyOrdering{BeforeSystemDefined: []string{"c", "a"}},
			current:  policyOrdering{BeforeSystemDefined: []string{"a", "b", "c"}, AfterSystemDefined: []string{"d"}},
			expected: policyOrdering{BeforeSystemDefined: []string{"c", "a", "b"}, AfterSystemDefined: []string{"d"}},
		},
		"move between sides": {
			want:     policyOrdering{BeforeSystemDefined: []string{"b"}, AfterSystemDefined: []string{"a"}},
			current:  policyOrdering{BeforeSystemDefined: []string{"a", "b"}},
			expected: policyOrdering{BeforeSystemDefined: []string{"b"}, AfterSystemDefined: []string{"a"}},
		},
		"empty pair": {
			expected: policyOrdering{BeforeSystemDefined: []string{}, AfterSystemDefined: []string{}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := completeOrdering(tc.want, tc.current); !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("got %+v, want %+v", got, tc.expected)
			}
		})
	}
}

func TestOrderingBodySendsEmptyLists(t *testing.T) {
	raw, err := json.Marshal(policyOrderingAPI{OrderedFirewallPolicyIDs: completeOrdering(policyOrdering{}, policyOrdering{})})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(raw), `{"orderedFirewallPolicyIds":{"afterSystemDefined":[],"beforeSystemDefined":[]}}`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

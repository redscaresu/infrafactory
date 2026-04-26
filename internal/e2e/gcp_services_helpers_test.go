package e2e

import (
	"reflect"
	"testing"
)

// TestCollectKeyParents pins the (parent SA, per-SA index) keying
// for IAM keys. The replace-allowed identity check relies on this
// to detect a recreate that lost or duplicated keys under one SA,
// or one that rebound a key to a different SA. The infrafactory
// IAM e2e scenario only provisions one key, so the multi-key
// branches are exercised here directly.
func TestCollectKeyParents(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state map[string]any
		want  []string
	}{
		{
			name:  "no keys",
			state: keyState(),
			want:  []string{},
		},
		{
			name: "single key, parent in serviceAccountEmail",
			state: keyState(
				map[string]any{"serviceAccountEmail": "ci@p.iam.gserviceaccount.com"},
			),
			want: []string{"sa=ci@p.iam.gserviceaccount.com/i=0"},
		},
		{
			name: "single key, parent only in name path",
			state: keyState(
				map[string]any{"name": "projects/p/serviceAccounts/ci@p.iam.gserviceaccount.com/keys/abcd"},
			),
			want: []string{"sa=ci@p.iam.gserviceaccount.com/i=0"},
		},
		{
			name: "two keys on same SA increment per-SA index",
			state: keyState(
				map[string]any{"serviceAccountEmail": "ci@p.iam.gserviceaccount.com"},
				map[string]any{"serviceAccountEmail": "ci@p.iam.gserviceaccount.com"},
			),
			want: []string{
				"sa=ci@p.iam.gserviceaccount.com/i=0",
				"sa=ci@p.iam.gserviceaccount.com/i=1",
			},
		},
		{
			name: "keys on different SAs each start at 0",
			state: keyState(
				map[string]any{"serviceAccountEmail": "ci@p.iam.gserviceaccount.com"},
				map[string]any{"serviceAccountEmail": "deploy@p.iam.gserviceaccount.com"},
			),
			want: []string{
				"sa=ci@p.iam.gserviceaccount.com/i=0",
				"sa=deploy@p.iam.gserviceaccount.com/i=0",
			},
		},
		{
			name: "mixed parent sources on same SA share the per-SA index",
			state: keyState(
				map[string]any{"serviceAccountEmail": "ci@p.iam.gserviceaccount.com"},
				map[string]any{"name": "projects/p/serviceAccounts/ci@p.iam.gserviceaccount.com/keys/abcd"},
			),
			want: []string{
				"sa=ci@p.iam.gserviceaccount.com/i=0",
				"sa=ci@p.iam.gserviceaccount.com/i=1",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := collectKeyParents(tc.state)
			if len(tc.want) == 0 && len(got) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("collectKeyParents = %v, want %v", got, tc.want)
			}
		})
	}
}

// keyState returns a fakegcp-shaped state with the given key items.
func keyState(items ...map[string]any) map[string]any {
	keys := make([]any, 0, len(items))
	for _, item := range items {
		keys = append(keys, item)
	}
	return map[string]any{
		"iam": map[string]any{
			"keys": keys,
		},
	}
}

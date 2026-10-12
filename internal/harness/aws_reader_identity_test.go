package harness

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyAWSReadIdentityChecksTheAccountAndAnyPrincipal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name               string
		account, principal string
		wantSent           int
		wantErr            []string
	}{
		{name: "right account and principal", account: testAWSAccount, principal: testAWSPrincipal, wantSent: 1},
		{name: "right account, no principal", account: testAWSAccount, wantSent: 1},
		{name: "wrong account", account: "210987654321", wantSent: 1,
			wantErr: []string{`account "123456789012"`, `--account "210987654321"`}},
		{name: "wrong account, right principal", account: "210987654321", principal: testAWSPrincipal, wantSent: 1,
			wantErr: []string{`account "123456789012"`, `--account "210987654321"`}},
		{name: "wrong principal", account: testAWSAccount, principal: "arn:aws:iam::123456789012:user/admin", wantSent: 1,
			wantErr: []string{`principal "` + testAWSPrincipal + `"`, `--principal "arn:aws:iam::123456789012:user/admin"`,
				"arn:aws:sts::<account>:assumed-role/"}},
		{name: "empty account", principal: testAWSPrincipal, wantSent: 0,
			wantErr: []string{"the expected account must be set"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inner := &recordingAWSDoer{body: getCallerIdentityXMLBody}
			wall, err := NewAWSReadWall("sts", inner)
			require.NoError(t, err)

			err = VerifyAWSReadIdentity(context.Background(), awsReadTestConfig(&recordingAWSDoer{}), wall, tc.account, tc.principal)

			assert.Len(t, inner.bodies, tc.wantSent)
			assert.Equal(t, tc.wantSent, wall.Calls())
			if tc.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, want := range tc.wantErr {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

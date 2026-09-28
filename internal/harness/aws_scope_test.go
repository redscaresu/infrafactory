package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testClaimHolder = "run-1@build-host.example:4242"
	otherHolder     = "run-0@other-host.example:99"
	testAccount     = "123456789012"
)

// fakeSSM is Parameter Store in a map, answering JSON 1.1 as SSM does.
// before, when set, sees each request first under the lock and answers it
// by returning a non-zero status. Every request body is recorded.
type fakeSSM struct {
	mu       sync.Mutex
	params   map[string]string
	before   func(f *fakeSSM, op string, in fakeSSMInput) (int, string)
	ops      []string
	requests []string
}

type fakeSSMInput struct {
	Name, Value string
	Overwrite   *bool
}

func newFakeSSM(params map[string]string) *fakeSSM {
	if params == nil {
		params = map[string]string{}
	}
	return &fakeSSM{params: params}
}

func (f *fakeSSM) Do(req *http.Request) (*http.Response, error) {
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var in fakeSSMInput
	if err := json.Unmarshal(payload, &in); err != nil {
		return nil, err
	}
	op := strings.TrimPrefix(req.Header.Get("X-Amz-Target"), "AmazonSSM.")

	f.mu.Lock()
	defer f.mu.Unlock()
	f.ops = append(f.ops, op)
	f.requests = append(f.requests, string(payload))
	status, body := 0, ""
	if f.before != nil {
		status, body = f.before(f, op, in)
	}
	if status == 0 {
		status, body = f.answer(op, in)
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/x-amz-json-1.1"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

func (f *fakeSSM) answer(op string, in fakeSSMInput) (int, string) {
	value, exists := f.params[in.Name]
	switch {
	case op == "PutParameter" && exists && !aws.ToBool(in.Overwrite):
		return ssmErrorAnswer("ParameterAlreadyExists")
	case op == "PutParameter":
		f.params[in.Name] = in.Value
		return http.StatusOK, `{"Version":1}`
	case !exists:
		return ssmErrorAnswer("ParameterNotFound")
	case op == "GetParameter":
		body, _ := json.Marshal(map[string]any{"Parameter": map[string]any{"Name": in.Name, "Type": "String", "Value": value, "Version": 1}})
		return http.StatusOK, string(body)
	case op == "DeleteParameter":
		delete(f.params, in.Name)
		return http.StatusOK, `{}`
	}
	return http.StatusNotImplemented, `{"__type":"NotImplemented","message":"` + op + `"}`
}

func ssmErrorAnswer(code string) (int, string) {
	status := http.StatusBadRequest
	if code == "InternalServerError" {
		status = http.StatusInternalServerError
	}
	return status, `{"__type":"` + code + `","message":"` + code + `"}`
}

func (f *fakeSSM) recorded() (ops, requests []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ops...), append([]string(nil), f.requests...)
}

// writes counts the requests that could change a parameter.
func (f *fakeSSM) writes() int {
	ops, _ := f.recorded()
	n := 0
	for _, op := range ops {
		if op == "PutParameter" || op == "DeleteParameter" {
			n++
		}
	}
	return n
}

func (f *fakeSSM) value(name string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.params[name]
	return v, ok
}

func assertNoOverwrite(t *testing.T, f *fakeSSM) {
	t.Helper()
	_, requests := f.recorded()
	for _, body := range requests {
		assert.NotContains(t, strings.ReplaceAll(body, " ", ""), `"Overwrite":true`)
	}
}

func scopeTestEnv(t *testing.T) map[string]string {
	t.Helper()
	env, err := AWSSealedEnv(validAWSCredFile(t), "eu-west-2")
	require.NoError(t, err)
	return env
}

// fakeDoerEndpoint is never dialled: the fake doers answer every request.
const fakeDoerEndpoint = "http://127.0.0.1:1"

func TestTakeAWSClaimTakesAFreeScope(t *testing.T) {
	t.Parallel()
	ssmDoer := newFakeSSM(nil)

	err := TakeAWSClaim(context.Background(), scopeTestEnv(t), ssmDoer, fakeDoerEndpoint, testClaimHolder)

	require.NoError(t, err)
	value, _ := ssmDoer.value(AWSClaimParameter)
	assert.Equal(t, testClaimHolder, value)
	_, requests := ssmDoer.recorded()
	require.Len(t, requests, 1)
	assert.Contains(t, requests[0], `"Overwrite":false`)
}

// A put that lands and answers 500 is retried by the SDK; the retry
// answers ParameterAlreadyExists, and the stored value decides.
func landsThenFails(f *fakeSSM, op string, in fakeSSMInput) (int, string) {
	if op == "PutParameter" && len(f.ops) == 1 {
		f.params[in.Name] = in.Value
		return ssmErrorAnswer("InternalServerError")
	}
	return 0, ""
}

func TestTakeAWSClaimResolvesAFailedPutByTheStoredValue(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		stored   map[string]string
		before   func(*fakeSSM, string, fakeSSMInput) (int, string)
		wantErr  error
		wantText string
		wantOps  []string
	}{
		{
			name:    "our retried put landed",
			before:  landsThenFails,
			wantOps: []string{"PutParameter", "PutParameter", "GetParameter"},
		},
		{
			name:     "another holder has it",
			stored:   map[string]string{AWSClaimParameter: otherHolder},
			wantErr:  ErrAWSScopeClaimed,
			wantText: otherHolder,
			wantOps:  []string{"PutParameter", "GetParameter"},
		},
		{
			name: "the follow-up get fails",
			before: func(f *fakeSSM, op string, in fakeSSMInput) (int, string) {
				if op == "GetParameter" {
					return ssmErrorAnswer("AccessDeniedException")
				}
				return landsThenFails(f, op, in)
			},
			wantErr:  ErrAWSClaimOutcomeUnknown,
			wantText: "AccessDeniedException",
			wantOps:  []string{"PutParameter", "PutParameter", "GetParameter"},
		},
		{
			name: "another put error, then the get fails",
			before: func(*fakeSSM, string, fakeSSMInput) (int, string) {
				return ssmErrorAnswer("AccessDeniedException")
			},
			wantErr: ErrAWSClaimOutcomeUnknown,
			wantOps: []string{"PutParameter", "GetParameter"},
		},
		{
			name: "another put error, and nothing is stored",
			before: func(_ *fakeSSM, op string, _ fakeSSMInput) (int, string) {
				if op == "PutParameter" {
					return ssmErrorAnswer("AccessDeniedException")
				}
				return 0, ""
			},
			wantText: "no claim is stored",
			wantOps:  []string{"PutParameter", "GetParameter"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ssmDoer := newFakeSSM(tc.stored)
			ssmDoer.before = tc.before

			err := TakeAWSClaim(context.Background(), scopeTestEnv(t), ssmDoer, fakeDoerEndpoint, testClaimHolder)

			ops, _ := ssmDoer.recorded()
			assert.Equal(t, tc.wantOps, ops)
			assertNoOverwrite(t, ssmDoer)
			if tc.wantErr == nil && tc.wantText == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NotErrorIs(t, err, ErrAWSScopeClaimed)
				assert.NotErrorIs(t, err, ErrAWSClaimOutcomeUnknown)
			}
			assert.Contains(t, err.Error(), tc.wantText)
			assert.NotContains(t, err.Error(), testAWSSecret)
		})
	}
}

func TestTakeAWSClaimIgnoresTheCallersCancellation(t *testing.T) {
	t.Parallel()
	ssmDoer := newFakeSSM(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := TakeAWSClaim(ctx, scopeTestEnv(t), ssmDoer, fakeDoerEndpoint, testClaimHolder)

	require.NoError(t, err)
	ops, _ := ssmDoer.recorded()
	assert.Equal(t, []string{"PutParameter"}, ops)
	value, _ := ssmDoer.value(AWSClaimParameter)
	assert.Equal(t, testClaimHolder, value)
}

func TestReadAWSClaimHolder(t *testing.T) {
	t.Parallel()
	env := scopeTestEnv(t)

	holder, held, err := ReadAWSClaimHolder(context.Background(), env, newFakeSSM(map[string]string{AWSClaimParameter: otherHolder}), fakeDoerEndpoint)
	require.NoError(t, err)
	assert.True(t, held)
	assert.Equal(t, otherHolder, holder)

	holder, held, err = ReadAWSClaimHolder(context.Background(), env, newFakeSSM(nil), fakeDoerEndpoint)
	require.NoError(t, err)
	assert.False(t, held)
	assert.Empty(t, holder)

	_, _, err = ReadAWSClaimHolder(context.Background(), env, newFakeSSM(map[string]string{AWSClaimParameter: "x; rm -rf ~"}), fakeDoerEndpoint)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a holder")
}

func TestReleaseAWSClaimDeletesOnlyItsOwnClaim(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		stored   map[string]string
		wantText string
	}{
		{name: "another holder", stored: map[string]string{AWSClaimParameter: otherHolder}, wantText: otherHolder},
		{name: "no claim", wantText: "no claim is held"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ssmDoer := newFakeSSM(tc.stored)

			err := ReleaseAWSClaim(context.Background(), scopeTestEnv(t), ssmDoer, fakeDoerEndpoint, testClaimHolder)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantText)
			assert.Zero(t, ssmDoer.writes())
		})
	}

	t.Run("its own claim", func(t *testing.T) {
		t.Parallel()
		ssmDoer := newFakeSSM(map[string]string{AWSClaimParameter: testClaimHolder})

		require.NoError(t, ReleaseAWSClaim(context.Background(), scopeTestEnv(t), ssmDoer, fakeDoerEndpoint, testClaimHolder))

		ops, _ := ssmDoer.recorded()
		assert.Equal(t, []string{"GetParameter", "DeleteParameter"}, ops)
		_, exists := ssmDoer.value(AWSClaimParameter)
		assert.False(t, exists)
	})

	t.Run("its own delete lands and the retry finds nothing", func(t *testing.T) {
		t.Parallel()
		ssmDoer := newFakeSSM(map[string]string{AWSClaimParameter: testClaimHolder})
		deletes := 0
		ssmDoer.before = func(f *fakeSSM, op string, in fakeSSMInput) (int, string) {
			if op == "DeleteParameter" {
				if deletes++; deletes == 1 {
					delete(f.params, in.Name)
					return ssmErrorAnswer("InternalServerError")
				}
			}
			return 0, ""
		}

		require.NoError(t, ReleaseAWSClaim(context.Background(), scopeTestEnv(t), ssmDoer, fakeDoerEndpoint, testClaimHolder))

		ops, _ := ssmDoer.recorded()
		assert.Equal(t, []string{"GetParameter", "DeleteParameter", "DeleteParameter"}, ops)
	})

	t.Run("a failed delete", func(t *testing.T) {
		t.Parallel()
		ssmDoer := newFakeSSM(map[string]string{AWSClaimParameter: testClaimHolder})
		ssmDoer.before = func(_ *fakeSSM, op string, _ fakeSSMInput) (int, string) {
			if op == "DeleteParameter" {
				return ssmErrorAnswer("AccessDeniedException")
			}
			return 0, ""
		}

		err := ReleaseAWSClaim(context.Background(), scopeTestEnv(t), ssmDoer, fakeDoerEndpoint, testClaimHolder)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "AccessDeniedException")
		value, _ := ssmDoer.value(AWSClaimParameter)
		assert.Equal(t, testClaimHolder, value)
	})
}

// deadLocalPID is the pid of a child that has exited and been reaped.
func deadLocalPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	require.NoError(t, cmd.Run())
	return cmd.Process.Pid
}

func TestTakeOverAWSClaim(t *testing.T) {
	t.Parallel()
	host, err := os.Hostname()
	require.NoError(t, err)
	liveLocal := fmt.Sprintf("run-0@%s:%d", host, os.Getpid())
	deadLocal := fmt.Sprintf("run-0@%s:%d", host, deadLocalPID(t))
	const intruder = "run-2@third-host.example:7"

	for _, tc := range []struct {
		name       string
		previous   string
		before     func(*fakeSSM, string, fakeSSMInput) (int, string)
		wantErr    error
		wantText   string
		wantWrites int
		wantValue  string
	}{
		{name: "another host's holder", previous: otherHolder, wantWrites: 2, wantValue: testClaimHolder},
		{name: "a dead local holder", previous: deadLocal, wantWrites: 2, wantValue: testClaimHolder},
		{name: "the stored value is not previous", previous: "run-5@other-host.example:5",
			wantErr: ErrAWSScopeClaimed, wantText: otherHolder, wantValue: otherHolder},
		{name: "a live local holder", previous: liveLocal, wantText: "still running", wantValue: liveLocal},
		{
			name: "another holder takes it after the delete", previous: otherHolder,
			before: func(f *fakeSSM, op string, in fakeSSMInput) (int, string) {
				if op == "DeleteParameter" {
					f.params[in.Name] = intruder
					return http.StatusOK, `{}`
				}
				return 0, ""
			},
			wantErr: ErrAWSScopeClaimed, wantText: intruder, wantWrites: 2, wantValue: intruder,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stored := otherHolder
			if tc.previous == liveLocal || tc.previous == deadLocal {
				stored = tc.previous
			}
			ssmDoer := newFakeSSM(map[string]string{AWSClaimParameter: stored})
			ssmDoer.before = tc.before

			err := TakeOverAWSClaim(context.Background(), scopeTestEnv(t), ssmDoer, fakeDoerEndpoint, tc.previous, testClaimHolder)

			if tc.wantErr == nil && tc.wantText == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				if tc.wantErr != nil {
					assert.ErrorIs(t, err, tc.wantErr)
				}
				assert.Contains(t, err.Error(), tc.wantText)
			}
			assert.Equal(t, tc.wantWrites, ssmDoer.writes())
			value, _ := ssmDoer.value(AWSClaimParameter)
			assert.Equal(t, tc.wantValue, value)
			assertNoOverwrite(t, ssmDoer)
		})
	}
}

func TestAWSClaimPrimitivesRefuseAMalformedHolderBeforeAnyRequest(t *testing.T) {
	t.Parallel()
	env := scopeTestEnv(t)
	ssmDoer := newFakeSSM(nil)
	const bad = "run 1@host:1"

	assert.Error(t, TakeAWSClaim(context.Background(), env, ssmDoer, fakeDoerEndpoint, bad))
	assert.Error(t, ReleaseAWSClaim(context.Background(), env, ssmDoer, fakeDoerEndpoint, bad))
	assert.Error(t, TakeOverAWSClaim(context.Background(), env, ssmDoer, fakeDoerEndpoint, bad, testClaimHolder))
	assert.Error(t, TakeOverAWSClaim(context.Background(), env, ssmDoer, fakeDoerEndpoint, otherHolder, bad))
	ops, _ := ssmDoer.recorded()
	assert.Empty(t, ops)
}

func TestNewAWSClaimHolder(t *testing.T) {
	t.Parallel()

	holder, err := NewAWSClaimHolder("20260928T101010Z-abc")
	require.NoError(t, err)
	assert.Regexp(t, awsClaimHolderRe, holder)
	assert.True(t, strings.HasPrefix(holder, "20260928T101010Z-abc@"))
	assert.True(t, strings.HasSuffix(holder, fmt.Sprintf(":%d", os.Getpid())))

	for _, runID := range []string{"run 1", `run"1`, "run'1", "", "run@1"} {
		_, err := NewAWSClaimHolder(runID)
		assert.Error(t, err, "run id %q", runID)
	}
}

func TestAssertAWSScopeStamp(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		stored   map[string]string
		wantText string
	}{
		{name: "missing", wantText: "docs/operations.md § Layer 3 (AWS)"},
		{name: "another account", stored: map[string]string{AWSStampParameter: "999999999999"}, wantText: `holds "999999999999", not the configured account "` + testAccount + `"`},
		{name: "matching", stored: map[string]string{AWSStampParameter: testAccount}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ssmDoer := newFakeSSM(tc.stored)

			err := AssertAWSScopeStamp(context.Background(), scopeTestEnv(t), ssmDoer, fakeDoerEndpoint, testAccount)

			if tc.wantText == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantText)
			}
			ops, _ := ssmDoer.recorded()
			assert.Equal(t, []string{"GetParameter"}, ops)
		})
	}

	t.Run("an empty account", func(t *testing.T) {
		t.Parallel()
		ssmDoer := newFakeSSM(nil)
		assert.Error(t, AssertAWSScopeStamp(context.Background(), scopeTestEnv(t), ssmDoer, fakeDoerEndpoint, ""))
		ops, _ := ssmDoer.recorded()
		assert.Empty(t, ops)
	})
}

// ec2VPCPages answers DescribeVpcs by NextToken: pages[""] is the first.
type ec2VPCPages struct {
	mu     sync.Mutex
	status int
	pages  map[string]string
	tokens []string
}

func (d *ec2VPCPages) Do(req *http.Request) (*http.Response, error) {
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	form, err := url.ParseQuery(string(payload))
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	token := form.Get("NextToken")
	d.tokens = append(d.tokens, token)
	status, body := http.StatusOK, d.pages[token]
	if d.status != 0 {
		status, body = d.status, ec2UnauthorizedBody
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"text/xml"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

func vpcsPage(nextToken string, vpcs ...string) string {
	page := `<DescribeVpcsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><vpcSet>` + strings.Join(vpcs, "") + `</vpcSet>`
	if nextToken != "" {
		page += `<nextToken>` + nextToken + `</nextToken>`
	}
	return page + `</DescribeVpcsResponse>`
}

func vpcItem(id string, isDefault bool) string {
	return fmt.Sprintf(`<item><vpcId>%s</vpcId><isDefault>%t</isDefault></item>`, id, isDefault)
}

func TestAssertNoAWSDefaultVPC(t *testing.T) {
	t.Parallel()

	t.Run("no default VPC on either page", func(t *testing.T) {
		t.Parallel()
		doer := &ec2VPCPages{pages: map[string]string{
			"":   vpcsPage("p2", vpcItem("vpc-0aaa", false)),
			"p2": vpcsPage("", vpcItem("vpc-0bbb", false)),
		}}
		require.NoError(t, AssertNoAWSDefaultVPC(context.Background(), scopeTestEnv(t), doer, fakeDoerEndpoint))
		assert.Equal(t, []string{"", "p2"}, doer.tokens)
	})

	t.Run("the default VPC is on page 2", func(t *testing.T) {
		t.Parallel()
		doer := &ec2VPCPages{pages: map[string]string{
			"":   vpcsPage("p2", vpcItem("vpc-0aaa", false)),
			"p2": vpcsPage("", vpcItem("vpc-0def", true)),
		}}
		err := AssertNoAWSDefaultVPC(context.Background(), scopeTestEnv(t), doer, fakeDoerEndpoint)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vpc-0def")
		assert.Equal(t, []string{"", "p2"}, doer.tokens)
	})

	t.Run("a 403 is an error", func(t *testing.T) {
		t.Parallel()
		doer := &ec2VPCPages{status: http.StatusForbidden}
		err := AssertNoAWSDefaultVPC(context.Background(), scopeTestEnv(t), doer, fakeDoerEndpoint)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "UnauthorizedOperation")
	})
}

// awsStampWrites names each function in file that both mentions
// AWSStampParameter and calls PutParameter or DeleteParameter. The stamp
// is written by hand at setup; infrafactory only reads it.
func awsStampWrites(file *ast.File) []string {
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}
		stamp, write := false, false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Ident:
				stamp = stamp || n.Name == "AWSStampParameter"
			case *ast.SelectorExpr:
				write = write || n.Sel.Name == "PutParameter" || n.Sel.Name == "DeleteParameter"
			}
			return true
		})
		if stamp && write {
			found = append(found, fn.Name.Name)
		}
		return false
	})
	return found
}

func TestNoHarnessCodeWritesTheAWSStamp(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	var violations []string
	mentions := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		ast.Inspect(file, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == "AWSStampParameter" {
				mentions++
			}
			return true
		})
		for _, fn := range awsStampWrites(file) {
			violations = append(violations, path+": "+fn)
		}
	}
	require.NotZero(t, mentions, "no harness code mentions AWSStampParameter; the audit is looking in the wrong place")
	assert.Empty(t, violations, "these functions pass the stamp to PutParameter or DeleteParameter; the stamp is written by hand at setup and only read here")
}

func TestAWSStampWritesAuditFixtures(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		want []string
	}{
		{"put", `package p
func f(c C) { c.PutParameter(ctx, &In{Name: aws.String(AWSStampParameter)}) }`, []string{"f"}},
		{"delete through a variable", `package p
func g(c C) { name := AWSStampParameter; c.DeleteParameter(ctx, &In{Name: &name}) }`, []string{"g"}},
		{"qualified", `package p
func h(c C) { c.DeleteParameter(ctx, &In{Name: aws.String(harness.AWSStampParameter)}) }`, []string{"h"}},
		{"read only", `package p
func r(c C) { c.GetParameter(ctx, &In{Name: aws.String(AWSStampParameter)}) }`, nil},
		{"claim delete", `package p
func d(c C) { c.DeleteParameter(ctx, &In{Name: aws.String(AWSClaimParameter)}) }`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", tc.src, 0)
			require.NoError(t, err)
			assert.Equal(t, tc.want, awsStampWrites(file))
		})
	}
}

// TestAWSScopeClaimAgainstRealSSM takes and releases the claim in the
// real Layer 3 account. It is run by hand and named in no workflow. Its
// env comes only from the credential file, never from shell AWS_*, and
// identity and stamp are checked before anything is written.
func TestAWSScopeClaimAgainstRealSSM(t *testing.T) {
	if os.Getenv("INFRAFACTORY_AWS_LAYER3_REAL") != "1" {
		t.Skip("set INFRAFACTORY_AWS_LAYER3_REAL=1 to claim the real AWS Layer 3 scope")
	}
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	env, err := AWSSealedEnv(filepath.Join(home, ".config", "infrafactory", "layer3-aws.env"), os.Getenv("INFRAFACTORY_AWS_LAYER3_REGION"))
	require.NoError(t, err)
	account := os.Getenv("INFRAFACTORY_AWS_LAYER3_ACCOUNT")
	doer := &http.Client{Timeout: 30 * time.Second}
	ctx := context.Background()

	require.NoError(t, VerifyAWSIdentity(ctx, env, doer, "", account, os.Getenv("INFRAFACTORY_AWS_LAYER3_PRINCIPAL")))
	require.NoError(t, AssertAWSScopeStamp(ctx, env, doer, "", account))

	holder, err := NewAWSClaimHolder("real-ssm-test")
	require.NoError(t, err)
	require.NoError(t, TakeAWSClaim(ctx, env, doer, "", holder))
	released := false
	t.Cleanup(func() {
		if !released {
			assert.NoError(t, ReleaseAWSClaim(ctx, env, doer, "", holder))
		}
	})

	err = TakeAWSClaim(ctx, env, doer, "", "real-ssm-second@other-host.example:1")
	require.ErrorIs(t, err, ErrAWSScopeClaimed)
	assert.Contains(t, err.Error(), holder)
	require.NoError(t, ReleaseAWSClaim(ctx, env, doer, "", holder))
	released = true

	client, err := newAWSSSMClient(env, doer, "")
	require.NoError(t, err)
	pages := ssm.NewDescribeParametersPaginator(client, &ssm.DescribeParametersInput{
		ParameterFilters: []ssmtypes.ParameterStringFilter{{
			Key: aws.String("Name"), Option: aws.String("BeginsWith"), Values: []string{AWSScopePrefix},
		}},
	})
	var names []string
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		require.NoError(t, err)
		for _, p := range page.Parameters {
			names = append(names, aws.ToString(p.Name))
		}
	}
	assert.Equal(t, []string{AWSStampParameter}, names)
}

package harness

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// The scrub (cmd/aws-read-scrub) turns a recorded read and the kept
// pre-destroy tfstate into fixtures fit for a public repo: the read's
// account id becomes AWSScrubAccount wherever it appears in either file,
// and each value below becomes its placeholder. CheckScrubbedRead then
// refuses anything the scrub did not hide.
const (
	// AWSScrubAccount is the synthetic account id.
	AWSScrubAccount = "111122223333"
	// AWSScrubUserData replaces DescribeInstanceAttribute's userData and
	// the state's user_data.
	AWSScrubUserData = "SCRUBBED-USER-DATA"
	// AWSScrubParameterValue replaces an SSM Parameter.Value outside
	// /aws/service/.
	AWSScrubParameterValue = "SCRUBBED-PARAMETER-VALUE"
	// AWSScrubUserID replaces GetCallerIdentity's UserId.
	AWSScrubUserID = "SCRUBBED-USER-ID"
	// AWSScrubRequestID replaces every RequestId: one whose last group is
	// twelve decimal digits would read as an account id.
	AWSScrubRequestID = "SCRUBBED-REQUEST-ID"
	// AWSScrubLineage replaces the state's lineage.
	AWSScrubLineage = "SCRUBBED-LINEAGE"
)

// The files WriteScrubbedAWSRead writes. The state is named .json, not
// .tfstate: .gitignore keeps every *.tfstate out of git, so raw state is
// never committed, and this file is meant to be.
const (
	AWSScrubbedReadFile  = "recorded-read.json"
	AWSScrubbedStateFile = "pre-destroy-state.json"
)

// awsPublicParameterPrefix holds the parameters AWS publishes, whose
// values carry no account (their ARN's account field is empty,
// aws_scope_policy_test.go), so the scrub keeps them.
const awsPublicParameterPrefix = "/aws/service/"

// awsStateUserDataKeys are the state attributes holding user data.
var awsStateUserDataKeys = []string{"user_data", "user_data_base64"}

// awsAccountIDDigits is the length of an AWS account id.
const awsAccountIDDigits = 12

var (
	awsAccountIDPattern = regexp.MustCompile(fmt.Sprintf(`^[0-9]{%d}$`, awsAccountIDDigits))
	awsDigitRunPattern  = regexp.MustCompile(`[0-9]+`)
	// awsUniqueIDPattern is an AWS unique id (AIDA…, AROA…) or key id
	// (AKIA…, ASIA…).
	awsUniqueIDPattern      = regexp.MustCompile(`A[A-Z]{3}[A-Z0-9]{12,}`)
	awsPercentEscapePattern = regexp.MustCompile(`%[0-9A-Fa-f]{2}`)
)

// WriteScrubbedAWSRead scrubs the recorded read at readPath and the
// tfstate at statePath, runs CheckScrubbedRead over the exact bytes it
// would write, and only once that passes writes them into outDir as
// AWSScrubbedReadFile and AWSScrubbedStateFile, mode 0600.
func WriteScrubbedAWSRead(readPath, statePath, outDir string) error {
	read, err := os.ReadFile(readPath)
	if err != nil {
		return err
	}
	state, err := os.ReadFile(statePath)
	if err != nil {
		return err
	}
	scrubbedRead, scrubbedState, err := scrubAWSRead(read, state)
	if err != nil {
		return err
	}
	readOut, stateOut := filepath.Join(outDir, AWSScrubbedReadFile), filepath.Join(outDir, AWSScrubbedStateFile)
	if err := CheckScrubbedRead(readOut, scrubbedRead, stateOut, scrubbedState); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return err
	}
	if err := writePrivateFile(readOut, scrubbedRead); err != nil {
		return err
	}
	return writePrivateFile(stateOut, scrubbedState)
}

// scrubAWSRead maps the read's account id to AWSScrubAccount in the raw
// bytes of both files, which reaches every form JSON writes a digit in
// (strings, keys, numbers, plain and URL-encoded ARNs), then puts the
// placeholders in. Its errors never quote either file.
func scrubAWSRead(read, state []byte) ([]byte, []byte, error) {
	var header struct {
		Account string `json:"account"`
	}
	if json.Unmarshal(read, &header) != nil || !awsAccountIDPattern.MatchString(header.Account) {
		return nil, nil, errors.New("the recorded read does not parse, or its account is not a 12-digit id")
	}
	mapAccount := func(data []byte) []byte {
		return bytes.ReplaceAll(data, []byte(header.Account), []byte(AWSScrubAccount))
	}
	scrubbedRead, err := scrubAWSRecordedRead(mapAccount(read))
	if err != nil {
		return nil, nil, fmt.Errorf("the recorded read: %w", err)
	}
	scrubbedState, err := scrubAWSState(mapAccount(state))
	if err != nil {
		return nil, nil, fmt.Errorf("the state: %w", err)
	}
	return scrubbedRead, scrubbedState, nil
}

func scrubAWSRecordedRead(data []byte) ([]byte, error) {
	var read AWSRecordedRead
	if decodeOneJSON(data, &read) != nil {
		return nil, errors.New("does not parse as an ADR-0041 recorded read")
	}
	for i := range read.Exchanges {
		body, err := scrubAWSReadBody(read.Exchanges[i])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", awsReadEntryName(i, read.Exchanges[i].AWSReadEntry), err)
		}
		read.Exchanges[i].Body = body
	}
	return encodeScrubbedJSON(read, "  ")
}

// scrubAWSReadBody is ex's body with its placeholders in: in XML every
// RequestId, DescribeInstanceAttribute's userData value and
// GetCallerIdentity's UserId; in SSM GetParameter's JSON the
// Parameter.Value, unless the parameter is public. Tag values stay.
func scrubAWSReadBody(ex AWSReadExchange) (string, error) {
	switch ex.Protocol {
	case awsProtocolQuery:
		return awsXMLReplaceText(ex.Body, func(names []string) (string, bool) {
			name := names[len(names)-1]
			switch {
			case strings.EqualFold(name, "RequestId"):
				return AWSScrubRequestID, true
			case ex.Action == "DescribeInstanceAttribute" && name == "value" && len(names) > 1 && names[len(names)-2] == "userData":
				return AWSScrubUserData, true
			case ex.Action == "GetCallerIdentity" && name == "UserId":
				return AWSScrubUserID, true
			}
			return "", false
		})
	case awsProtocolJSON:
		if ex.Service != "ssm" || ex.Action != "GetParameter" {
			return ex.Body, nil
		}
		return scrubAWSParameterValue(ex.Body)
	}
	return "", errors.New("its protocol is neither query nor json")
}

func scrubAWSParameterValue(body string) (string, error) {
	var answer map[string]any
	if decodeOneJSON([]byte(body), &answer) != nil {
		return "", errors.New("the body does not parse as JSON")
	}
	parameter, _ := answer["Parameter"].(map[string]any)
	if _, ok := parameter["Value"]; !ok || awsPublicParameter(parameter["Name"]) {
		return body, nil
	}
	parameter["Value"] = AWSScrubParameterValue
	out, err := encodeScrubbedJSON(answer, "")
	return string(bytes.TrimSuffix(out, []byte("\n"))), err
}

// awsXMLReplaceText is body with the content of every element that
// placeholder names, given the names of the elements open down to it,
// replaced by the placeholder, and every other byte as it was. A
// self-closing element holds nothing and is left as it is.
func awsXMLReplaceText(body string, placeholder func(names []string) (string, bool)) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(body))
	var (
		out         strings.Builder
		names       []string
		copied      int64 // body[:copied] is in out
		open        int   // depth of the element being replaced, 0 for none
		from        int64 // where its content starts
		replacement string
	)
	for {
		at := dec.InputOffset()
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out.String() + body[copied:], nil
		}
		if err != nil {
			return "", errors.New("the body does not parse as XML")
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			names = append(names, tok.Name.Local)
			if v, ok := placeholder(names); ok && open == 0 {
				open, from, replacement = len(names), dec.InputOffset(), v
			}
		case xml.EndElement:
			if open == len(names) {
				if !strings.HasSuffix(body[:from], "/>") {
					out.WriteString(body[copied:from] + replacement)
					copied = at
				}
				open = 0
			}
			names = names[:len(names)-1]
		}
	}
}

// scrubAWSState replaces the state's lineage and every user_data and
// user_data_base64 string, keeping every key and every other value.
func scrubAWSState(data []byte) ([]byte, error) {
	var state map[string]any
	if decodeOneJSON(data, &state) != nil || state == nil {
		return nil, errors.New("does not parse as a JSON object")
	}
	if _, ok := state["lineage"]; ok {
		state["lineage"] = AWSScrubLineage
	}
	scrubAWSStateUserData(state)
	return encodeScrubbedJSON(state, "  ")
}

func scrubAWSStateUserData(v any) {
	switch v := v.(type) {
	case map[string]any:
		for key, value := range v {
			if _, ok := value.(string); ok && slices.Contains(awsStateUserDataKeys, key) {
				v[key] = AWSScrubUserData
				continue
			}
			scrubAWSStateUserData(value)
		}
	case []any:
		for _, item := range v {
			scrubAWSStateUserData(item)
		}
	}
}

// CheckScrubbedRead is the scrub's leak check, run over the exact bytes of
// the scrubbed read and state, each whole: by the scrub before it writes
// either, and by the committed fixtures' test. It refuses
//   - a 12-digit number other than AWSScrubAccount, where a hit is a
//     maximal run of exactly twelve digits;
//   - an AWS unique or key id (A[A-Z]{3}[A-Z0-9]{12,});
//   - a userData, an SSM Value or a state's private aws_ssm_parameter
//     value that is not its placeholder (nor, for a Value, a public
//     /aws/service/ parameter's);
//   - a read whose account is not AWSScrubAccount, and an object that
//     repeats a key, which a decode would hide.
//
// Text is read as written and percent-decoded, and a body as its XML or
// JSON decodes. The error names the file and the JSON path, never the
// value found.
func CheckScrubbedRead(readName string, read []byte, stateName string, state []byte) error {
	if err := checkScrubbedAWSRead(read); err != nil {
		return fmt.Errorf("scrub check: %s: %w", readName, err)
	}
	if err := checkScrubbedAWSState(state); err != nil {
		return fmt.Errorf("scrub check: %s: %w", stateName, err)
	}
	return nil
}

func checkScrubbedAWSRead(data []byte) error {
	if err := checkScrubbedJSON(data); err != nil {
		return err
	}
	var read AWSRecordedRead
	if decodeOneJSON(data, &read) != nil {
		return errors.New("$: does not parse as an ADR-0041 recorded read")
	}
	if read.Account != AWSScrubAccount {
		return errors.New("$.account: is not the synthetic account id")
	}
	for i, ex := range read.Exchanges {
		var err error
		switch ex.Protocol {
		case awsProtocolQuery:
			err = checkScrubbedXML(ex.Body)
		case awsProtocolJSON:
			err = checkScrubbedJSONBody(ex.Body)
		default:
			err = errors.New("its protocol is neither query nor json")
		}
		if err != nil {
			return fmt.Errorf("$.exchanges[%d].body: %w", i, err)
		}
	}
	return nil
}

func checkScrubbedAWSState(data []byte) error {
	if err := checkScrubbedJSON(data); err != nil {
		return err
	}
	var state any
	if decodeOneJSON(data, &state) != nil {
		return errors.New("$: does not parse as JSON")
	}
	return checkScrubbedStateValues("$", state)
}

// checkScrubbedJSON checks every key, string and number of data, which
// must hold exactly one JSON value, reading it token by token so that no
// repeated key hides a value.
func checkScrubbedJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := checkScrubbedJSONValue(dec, "$"); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("$: does not hold exactly one JSON value")
	}
	return nil
}

func checkScrubbedJSONValue(dec *json.Decoder, path string) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("%s: does not parse as JSON", path)
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		text, _ := tok.(string)
		if number, ok := tok.(json.Number); ok {
			text = number.String()
		}
		return checkScrubbedTextAt(path, text)
	}
	seen := map[string]bool{}
	for i := 0; dec.More(); i++ {
		member := fmt.Sprintf("%s[%d]", path, i)
		if delim == '{' {
			tok, err := dec.Token()
			key, _ := tok.(string)
			if err != nil {
				return fmt.Errorf("%s: does not parse as JSON", path)
			}
			// The key is checked before it is named in any path.
			if err := checkScrubbedTextAt(path+" (a key)", key); err != nil {
				return err
			}
			if seen[key] {
				return fmt.Errorf("%s.%s: repeats a key", path, key)
			}
			seen[key], member = true, path+"."+key
		}
		if err := checkScrubbedJSONValue(dec, member); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("%s: does not parse as JSON", path)
	}
	return nil
}

func checkScrubbedTextAt(path, text string) error {
	if err := checkScrubbedText(text); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// checkScrubbedText refuses text holding a 12-digit number other than
// AWSScrubAccount or an AWS unique or key id, as written or
// percent-decoded: IAM writes policy documents URL-encoded, where %22
// glues digits onto an account id.
func checkScrubbedText(text string) error {
	for _, view := range awsPercentDecodings(text) {
		for _, loc := range awsDigitRunPattern.FindAllStringIndex(view, -1) {
			run := view[loc[0]:loc[1]]
			if len(run) == awsAccountIDDigits && run != AWSScrubAccount && !awsExponentFraction(view, loc[0], loc[1]) {
				return errors.New("holds a 12-digit number other than the synthetic account id")
			}
		}
		if awsUniqueIDPattern.MatchString(view) {
			return errors.New("holds an AWS unique or key id")
		}
	}
	return nil
}

// awsPercentDecodings is text, then text percent-decoded again and again
// until nothing is left to decode.
func awsPercentDecodings(text string) []string {
	views := []string{text}
	for {
		last := views[len(views)-1]
		next := awsPercentEscapePattern.ReplaceAllStringFunc(last, func(escape string) string {
			b, _ := hex.DecodeString(escape[1:])
			return string(b)
		})
		if next == last {
			return views
		}
		views = append(views, next)
	}
}

// awsExponentFraction reports whether the digits s[start:end] are the
// fraction of a number in exponent notation with a one-digit integer
// part (d.dddE9): SSM writes LastModifiedDate so, epoch seconds to the
// millisecond, twelve digits after the point.
func awsExponentFraction(s string, start, end int) bool {
	digit := func(i int) bool { return i >= 0 && i < len(s) && s[i] >= '0' && s[i] <= '9' }
	return start >= 2 && s[start-1] == '.' && digit(start-2) && !digit(start-3) &&
		end < len(s) && (s[end] == 'e' || s[end] == 'E')
}

// checkScrubbedXML checks an XML body's text and attribute values as they
// decode (entities and CDATA read as what they spell, text joined across
// comments), and refuses a userData whose text is neither empty nor
// AWSScrubUserData.
func checkScrubbedXML(body string) error {
	dec := xml.NewDecoder(strings.NewReader(body))
	var text, userData strings.Builder
	inUserData := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return checkScrubbedText(text.String())
		}
		if err != nil {
			return errors.New("does not parse as XML")
		}
		switch tok := tok.(type) {
		case xml.CharData:
			text.Write(tok)
			if inUserData > 0 {
				userData.Write(tok)
			}
			continue
		case xml.StartElement:
			for _, attr := range tok.Attr {
				if err := checkScrubbedText(attr.Value); err != nil {
					return err
				}
			}
			if strings.EqualFold(tok.Name.Local, "userData") {
				inUserData++
			}
		case xml.EndElement:
			if strings.EqualFold(tok.Name.Local, "userData") {
				inUserData--
				if v := strings.TrimSpace(userData.String()); inUserData == 0 && v != "" && v != AWSScrubUserData {
					return errors.New("holds a userData that is not the placeholder")
				}
			}
			if inUserData == 0 {
				userData.Reset()
			}
		default:
			continue
		}
		if err := checkScrubbedText(text.String()); err != nil {
			return err
		}
		text.Reset()
	}
}

// checkScrubbedJSONBody checks a json body's every key, string and number,
// and refuses a Value, in any object, that is neither
// AWSScrubParameterValue nor a public parameter's.
func checkScrubbedJSONBody(body string) error {
	if err := checkScrubbedJSON([]byte(body)); err != nil {
		return err
	}
	var answer any
	if decodeOneJSON([]byte(body), &answer) != nil {
		return errors.New("does not parse as JSON")
	}
	return walkJSONObjects("$", answer, func(path string, object map[string]any) error {
		if value, ok := object["Value"]; ok && value != AWSScrubParameterValue && !awsPublicParameter(object["Name"]) {
			return fmt.Errorf("%s.Value: is neither the placeholder nor a public parameter's value", path)
		}
		return nil
	})
}

// checkScrubbedStateValues refuses a user_data or user_data_base64 that is
// neither null nor AWSScrubUserData, and an aws_ssm_parameter outside
// /aws/service/ whose value or insecure_value is neither null nor
// AWSScrubParameterValue: the scrub replaces no such value, so a state
// holding one is refused.
func checkScrubbedStateValues(path string, state any) error {
	return walkJSONObjects(path, state, func(path string, object map[string]any) error {
		for _, key := range awsStateUserDataKeys {
			if value := object[key]; value != nil && value != AWSScrubUserData {
				return fmt.Errorf("%s.%s: is neither null nor the placeholder", path, key)
			}
		}
		if object["type"] != "aws_ssm_parameter" {
			return nil
		}
		instances, _ := object["instances"].([]any)
		for i, instance := range instances {
			fields, _ := instance.(map[string]any)
			attributes, _ := fields["attributes"].(map[string]any)
			for _, key := range []string{"value", "insecure_value"} {
				if value := attributes[key]; value != nil && value != AWSScrubParameterValue && !awsPublicParameter(attributes["name"]) {
					return fmt.Errorf("%s.instances[%d].attributes.%s: is a private SSM parameter's value", path, i, key)
				}
			}
		}
		return nil
	})
}

// walkJSONObjects calls visit with every object in v and its path.
func walkJSONObjects(path string, v any, visit func(path string, object map[string]any) error) error {
	switch v := v.(type) {
	case map[string]any:
		if err := visit(path, v); err != nil {
			return err
		}
		for _, key := range slices.Sorted(maps.Keys(v)) {
			if err := walkJSONObjects(path+"."+key, v[key], visit); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range v {
			if err := walkJSONObjects(fmt.Sprintf("%s[%d]", path, i), item, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

func awsPublicParameter(name any) bool {
	s, ok := name.(string)
	return ok && strings.HasPrefix(s, awsPublicParameterPrefix)
}

// decodeOneJSON decodes data, which must hold exactly one JSON value, into
// v, keeping numbers as written and refusing a field v lacks. Its error
// can quote data, so callers do not pass it on.
func decodeOneJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("more than one JSON value")
	}
	return nil
}

// encodeScrubbedJSON is v as JSON, leaving <, > and & unescaped so that a
// committed XML body stays legible.
func encodeScrubbedJSON(v any, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", indent)
	err := enc.Encode(v)
	return buf.Bytes(), err
}

// writePrivateFile writes data to path through a temp file renamed over
// it, so the file is mode 0600 even where one was there with a wider mode.
func writePrivateFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

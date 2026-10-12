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
	// AWSScrubUserID replaces GetCallerIdentity's UserId and
	// aws_caller_identity's user_id in the state.
	AWSScrubUserID = "SCRUBBED-USER-ID"
	// AWSScrubRequestID replaces every RequestId: one whose last group is
	// twelve decimal digits would read as an account id.
	AWSScrubRequestID = "SCRUBBED-REQUEST-ID"
	// AWSScrubLineage replaces the state's lineage.
	AWSScrubLineage = "SCRUBBED-LINEAGE"
	// AWSScrubPrincipal replaces the user name, or the role and session
	// names, after user/ or assumed-role/ in every IAM user's or assumed
	// role's ARN in either file.
	AWSScrubPrincipal = "SCRUBBED-PRINCIPAL"
	// AWSScrubSensitiveValue replaces every state attribute that a resource
	// instance lists in sensitive_attributes (but a public SSM parameter's
	// value) and every sensitive output's value.
	AWSScrubSensitiveValue = "SCRUBBED-SENSITIVE-VALUE"
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

// awsSSMValueKeys are an aws_ssm_parameter's attributes holding its value
// in the state.
var awsSSMValueKeys = []string{"value", "insecure_value"}

// awsAccountIDDigits is the length of an AWS account id.
const awsAccountIDDigits = 12

var (
	awsAccountIDPattern = regexp.MustCompile(fmt.Sprintf(`^[0-9]{%d}$`, awsAccountIDDigits))
	awsDigitRunPattern  = regexp.MustCompile(`[0-9]+`)
	// awsUniqueIDPattern is an AWS unique id (AIDA…, AROA…) or key id
	// (AKIA…, ASIA…).
	awsUniqueIDPattern      = regexp.MustCompile(`A[A-Z]{3}[A-Z0-9]{12,}`)
	awsPercentEscapePattern = regexp.MustCompile(`%[0-9A-Fa-f]{2}`)
	// awsPrincipalARNPattern is an IAM user's or an assumed role's ARN:
	// group 1 runs up to the name, group 2 is the name (a user's path and
	// name, or a role and session).
	awsPrincipalARNPattern = regexp.MustCompile(fmt.Sprintf(`(%s)(%s)`, fmt.Sprintf(awsPrincipalARNPrefix, `[0-9]{12}`), awsPrincipalNameChars))
	// awsScrubbedPrincipalARNPattern is the only ARN GetCallerIdentity may
	// answer with once scrubbed.
	awsScrubbedPrincipalARNPattern = regexp.MustCompile(fmt.Sprintf(`^%s%s$`, fmt.Sprintf(awsPrincipalARNPrefix, AWSScrubAccount), AWSScrubPrincipal))
	// awsScientificNumberPattern is a JSON number in exponent notation with
	// a one-digit integer part, as SSM writes LastModifiedDate: epoch
	// seconds to the millisecond, twelve digits after the point.
	awsScientificNumberPattern = regexp.MustCompile(`^-?[0-9]\.[0-9]+[eE][+-]?[0-9]+$`)
	// awsRecordedBodyPathPattern is the path of an exchange's body in a
	// recorded read.
	awsRecordedBodyPathPattern = regexp.MustCompile(`^\$\.exchanges\[[0-9]+\]\.body$`)
)

// ValidAWSAccountID reports whether id is an AWS account id: twelve digits.
func ValidAWSAccountID(id string) bool {
	return awsAccountIDPattern.MatchString(id)
}

// awsPrincipalARNPrefix is a principal's ARN up to its name, around an
// account pattern; awsPrincipalNameChars is what IAM allows in a name
// ([\w+=,.@-]), plus / for a user's path and between role and session.
const (
	awsPrincipalARNPrefix = `arn:aws:(?:iam|sts)::%s:(?:user|assumed-role)/`
	awsPrincipalNameChars = `[\w+=,.@/-]+`
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

// scrubAWSRead maps, in the raw bytes of both files, the read's account id
// to AWSScrubAccount, which reaches every form JSON writes a digit in
// (strings, keys, numbers, plain and URL-encoded ARNs), and the name in
// every IAM user's or assumed role's ARN to AWSScrubPrincipal (a
// URL-encoded one is left for the check to refuse). It then puts the
// placeholders in. Its errors never quote either file.
func scrubAWSRead(read, state []byte) ([]byte, []byte, error) {
	var header struct {
		Account string `json:"account"`
	}
	if json.Unmarshal(read, &header) != nil || !awsAccountIDPattern.MatchString(header.Account) {
		return nil, nil, errors.New("the recorded read does not parse, or its account is not a 12-digit id")
	}
	mapIdentity := func(data []byte) []byte {
		data = bytes.ReplaceAll(data, []byte(header.Account), []byte(AWSScrubAccount))
		return awsPrincipalARNPattern.ReplaceAll(data, []byte("${1}"+AWSScrubPrincipal))
	}
	scrubbedRead, err := scrubAWSRecordedRead(mapIdentity(read))
	if err != nil {
		return nil, nil, fmt.Errorf("the recorded read: %w", err)
	}
	scrubbedState, err := scrubAWSState(mapIdentity(state))
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
			switch {
			case strings.EqualFold(names[len(names)-1], "RequestId"):
				return AWSScrubRequestID, true
			case ex.Action == "DescribeInstanceAttribute" && awsXMLPathEnds(names, "userData", "value"):
				return AWSScrubUserData, true
			case awsXMLPathEnds(names, "GetCallerIdentityResult", "UserId"):
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
// replaced by the placeholder, and every other byte as it was. Elements
// are decided as they close, so an element holding one already replaced
// is left; a self-closing one holds nothing and is left too.
func awsXMLReplaceText(body string, placeholder func(names []string) (string, bool)) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(body))
	var (
		out    strings.Builder
		names  []string
		starts []int64 // where each open element's content starts
		copied int64   // body[:copied] is in out
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
			names, starts = append(names, tok.Name.Local), append(starts, dec.InputOffset())
		case xml.EndElement:
			from := starts[len(starts)-1]
			if from >= copied && !strings.HasSuffix(body[:from], "/>") {
				if v, ok := placeholder(names); ok {
					out.WriteString(body[copied:from] + v)
					copied = at
				}
			}
			names, starts = names[:len(names)-1], starts[:len(starts)-1]
		}
	}
}

// awsXMLPathEnds reports whether the open elements names end in suffix.
func awsXMLPathEnds(names []string, suffix ...string) bool {
	return len(names) >= len(suffix) && slices.Equal(names[len(names)-len(suffix):], suffix)
}

// scrubAWSState replaces the state's lineage, every user_data and
// user_data_base64 string, every attribute a resource instance lists in
// sensitive_attributes (but a public SSM parameter's value), every
// sensitive output's value and aws_caller_identity's user_id, keeping
// every key and every other value.
func scrubAWSState(data []byte) ([]byte, error) {
	var state map[string]any
	if decodeOneJSON(data, &state) != nil || state == nil {
		return nil, errors.New("does not parse as a JSON object")
	}
	if _, ok := state["lineage"]; ok {
		state["lineage"] = AWSScrubLineage
	}
	scrubAWSStateUserData(state)
	outputs, _ := state["outputs"].(map[string]any)
	for _, item := range outputs {
		if output, _ := item.(map[string]any); awsSensitiveOutput(output) && output["value"] != nil {
			output["value"] = AWSScrubSensitiveValue
		}
	}
	err := walkAWSStateInstances(state, func(_, resourceType string, attributes map[string]any, sensitive []string) error {
		for _, name := range sensitive {
			if attributes[name] != nil && !awsPublicStateValue(resourceType, attributes, name) {
				attributes[name] = AWSScrubSensitiveValue
			}
		}
		if resourceType == "aws_caller_identity" && attributes["user_id"] != nil {
			attributes["user_id"] = AWSScrubUserID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return encodeScrubbedJSON(state, "  ")
}

// awsSensitiveOutput reports whether a state output is marked sensitive:
// anything but an absent, null or false "sensitive" counts.
func awsSensitiveOutput(output map[string]any) bool {
	return output["sensitive"] != nil && output["sensitive"] != false
}

// walkAWSStateInstances calls visit with every resource instance in state:
// its path, its resource's type, its attributes, and the top-level
// attributes its sensitive_attributes paths start at.
func walkAWSStateInstances(state any, visit func(path, resourceType string, attributes map[string]any, sensitive []string) error) error {
	return walkJSONObjects("$", state, func(path string, object map[string]any) error {
		instances, ok := object["instances"].([]any)
		if !ok {
			return nil
		}
		resourceType, _ := object["type"].(string)
		for i, item := range instances {
			path := fmt.Sprintf("%s.instances[%d]", path, i)
			instance, _ := item.(map[string]any)
			attributes, _ := instance["attributes"].(map[string]any)
			sensitive, err := awsSensitiveAttributes(instance["sensitive_attributes"])
			if err != nil {
				return fmt.Errorf("%s.sensitive_attributes: %w", path, err)
			}
			if err := visit(path, resourceType, attributes, sensitive); err != nil {
				return err
			}
		}
		return nil
	})
}

// awsSensitiveAttributes is the top-level attribute each path in listed
// (an instance's sensitive_attributes, [[{"type":"get_attr","value":name},
// ...], ...]) starts at. The scrub hides the whole top-level attribute: a
// path into a nested block can index a set, which it does not follow.
func awsSensitiveAttributes(listed any) ([]string, error) {
	if listed == nil {
		return nil, nil
	}
	paths, ok := listed.([]any)
	if !ok {
		return nil, errors.New("is not a list of paths")
	}
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		var step map[string]any
		if steps, _ := path.([]any); len(steps) > 0 {
			step, _ = steps[0].(map[string]any)
		}
		name, ok := step["value"].(string)
		if !ok || step["type"] != "get_attr" {
			return nil, errors.New("holds a path that does not start at an attribute")
		}
		names = append(names, name)
	}
	return names, nil
}

// awsPublicStateValue reports whether the attribute name holds an SSM
// parameter's value under /aws/service/, which the provider marks
// sensitive but which carries no account. Every other attribute of that
// parameter follows the general rule.
func awsPublicStateValue(resourceType string, attributes map[string]any, name string) bool {
	return resourceType == "aws_ssm_parameter" && slices.Contains(awsSSMValueKeys, name) && awsPublicParameter(attributes["name"])
}

// awsHiddenStateValue reports whether a state value hides nothing.
func awsHiddenStateValue(v any) bool {
	return v == nil || v == AWSScrubSensitiveValue
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
//     maximal run of exactly twelve digits in a key, a string, or a JSON
//     number not in d.ddd…E9 form (awsScientificNumberPattern), but one of
//     awsPublicImageOwners as an EC2 DescribeImages item's imageOwnerId;
//   - an AWS unique or key id (A[A-Z]{3}[A-Z0-9]{12,});
//   - an IAM user's or assumed role's ARN, anywhere, whose name is not
//     AWSScrubPrincipal, and a GetCallerIdentity Arn of any other shape;
//   - a userData, an SSM Value, a state's user_data, a sensitive
//     output's value, or a state attribute listed in
//     sensitive_attributes or holding a private aws_ssm_parameter's
//     value, that is not its placeholder (nor, for an SSM value, a
//     public /aws/service/ parameter's);
//   - an XML comment, directive, or processing instruction other than
//     the declaration: a body answers with elements only;
//   - a read whose account is not AWSScrubAccount, and an object that
//     repeats a key (in the read, in any case), which a decode would
//     hide.
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
	// Bodies are checked below, each by its protocol: a raw SSM body holds
	// LastModifiedDate's twelve-digit fraction, which only its own JSON
	// reads as a number. The decode refuses an unknown field, so the
	// only strings at a body's path are the exchanges' bodies.
	if err := (awsJSONCheck{foldKeys: true, skip: awsRecordedBodyPathPattern.MatchString}).check(data); err != nil {
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
			// Raw, for element and attribute names; decoded, for text.
			if err = checkScrubbedText(awsMaskPublicImageOwners(ex)); err == nil {
				err = checkScrubbedXML(ex.Body, awsImageOwnerExchange(ex))
			}
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
	if err := (awsJSONCheck{}).check(data); err != nil {
		return err
	}
	var state any
	if decodeOneJSON(data, &state) != nil {
		return errors.New("$: does not parse as JSON")
	}
	if err := checkScrubbedStateOutputs(state); err != nil {
		return err
	}
	return checkScrubbedStateValues(state)
}

// awsJSONCheck runs checkScrubbedText over every key, string and number
// of one JSON value, token by token so that no repeated key hides a
// value. With foldKeys, keys repeat as encoding/json matches them to
// struct fields, ignoring case: for the read and its bodies, which Go
// decodes into structs. A state's keys repeat only when exact, since its
// tags may hold both Name and name. skip passes over a string at a path
// that another rule checks.
type awsJSONCheck struct {
	foldKeys bool
	skip     func(path string) bool
}

func (c awsJSONCheck) check(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := c.value(dec, "$"); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("$: does not hold exactly one JSON value")
	}
	return nil
}

func (c awsJSONCheck) value(dec *json.Decoder, path string) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("%s: does not parse as JSON", path)
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		text, _ := tok.(string)
		if c.skip != nil && c.skip(path) {
			return nil
		}
		if number, ok := tok.(json.Number); ok {
			if awsScientificNumberPattern.MatchString(number.String()) {
				return nil
			}
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
			id := key
			if c.foldKeys {
				// encoding/json's fold: each rune lowered, then raised.
				id = strings.ToUpper(strings.ToLower(key))
			}
			if seen[id] {
				return fmt.Errorf("%s.%s: repeats a key", path, key)
			}
			seen[id], member = true, path+"."+key
		}
		if err := c.value(dec, member); err != nil {
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
// AWSScrubAccount, an AWS unique or key id, or an IAM user's or assumed
// role's ARN whose name is not AWSScrubPrincipal, as written or
// percent-decoded: IAM writes policy documents URL-encoded, where %22
// glues digits onto an account id.
func checkScrubbedText(text string) error {
	for _, view := range awsPercentDecodings(text) {
		for _, run := range awsDigitRunPattern.FindAllString(view, -1) {
			if len(run) == awsAccountIDDigits && run != AWSScrubAccount {
				return errors.New("holds a 12-digit number other than the synthetic account id")
			}
		}
		if awsUniqueIDPattern.MatchString(view) {
			return errors.New("holds an AWS unique or key id")
		}
		for _, m := range awsPrincipalARNPattern.FindAllStringSubmatch(view, -1) {
			if m[2] != AWSScrubPrincipal {
				return errors.New("names an IAM user or assumed role other than the placeholder")
			}
		}
	}
	return nil
}

// awsPublicImageOwners are the accounts that own public images a read
// may hold, each with the AWS documentation naming it. Only these may
// stand, and only as an EC2 DescribeImages item's imageOwnerId: a public
// parameter's AMI is read (awsReplay.readSSM), and its owner is not the
// read's account, so the scrub leaves it.
var awsPublicImageOwners = []string{
	// Amazon, owner of the Amazon Linux AMIs, whose AL2023 release topic is
	// arn:aws:sns:us-east-1:137112412989:amazon-linux-2023-ami-updates:
	// https://docs.aws.amazon.com/linux/al2023/ug/receive-update-notification.html
	"137112412989",
}

// awsImageOwnerPath is where a DescribeImages answer names an image's
// owner, from the body's root.
var awsImageOwnerPath = []string{"DescribeImagesResponse", "imagesSet", "item", "imageOwnerId"}

// awsImageOwnerExchange reports whether ex is the one exchange that may
// name a public image owner: an EC2 DescribeImages.
func awsImageOwnerExchange(ex AWSReadExchange) bool {
	return ex.Service == "ec2" && ex.Action == "DescribeImages"
}

// awsMaskPublicImageOwners is ex's body for the raw check: in an
// awsImageOwnerExchange, each imageOwnerId element holding exactly a
// public owner id, and nothing else, is emptied. checkScrubbedXML checks
// the decoded text in its place, by its path.
func awsMaskPublicImageOwners(ex AWSReadExchange) string {
	if !awsImageOwnerExchange(ex) {
		return ex.Body
	}
	name := awsImageOwnerPath[len(awsImageOwnerPath)-1]
	open, end := "<"+name+">", "</"+name+">"
	body := ex.Body
	for _, owner := range awsPublicImageOwners {
		body = strings.ReplaceAll(body, open+owner+end, open+end)
	}
	return body
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

// checkScrubbedXML checks an XML body's text and attribute values as they
// decode (entities and CDATA read as what they spell), refuses a comment,
// a directive or a processing instruction but the XML declaration, a
// userData whose text is neither empty nor AWSScrubUserData, and a
// GetCallerIdentity Arn whose name is not AWSScrubPrincipal. With
// imageOwners (awsImageOwnerExchange), it passes the text of an
// imageOwnerId at awsImageOwnerPath that is one of awsPublicImageOwners.
func checkScrubbedXML(body string, imageOwners bool) error {
	dec := xml.NewDecoder(strings.NewReader(body))
	var text, userData strings.Builder
	var names []string
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
		case xml.Comment, xml.Directive:
			return errors.New("holds an XML comment or directive")
		case xml.ProcInst:
			if tok.Target != "xml" {
				return errors.New("holds a processing instruction other than the XML declaration")
			}
			if err := checkScrubbedText(string(tok.Inst)); err != nil {
				return err
			}
			continue
		case xml.StartElement:
			names = append(names, tok.Name.Local)
			for _, attr := range tok.Attr {
				if err := checkScrubbedText(attr.Value); err != nil {
					return err
				}
			}
			if strings.EqualFold(tok.Name.Local, "userData") {
				inUserData++
			}
		case xml.EndElement:
			if awsXMLPathEnds(names, "GetCallerIdentityResult", "Arn") && !awsScrubbedPrincipalARNPattern.MatchString(strings.TrimSpace(text.String())) {
				return errors.New("holds a GetCallerIdentity Arn whose name is not the placeholder")
			}
			if imageOwners && slices.Equal(names, awsImageOwnerPath) && slices.Contains(awsPublicImageOwners, text.String()) {
				text.Reset()
			}
			names = names[:len(names)-1]
			if strings.EqualFold(tok.Name.Local, "userData") {
				inUserData--
				if v := strings.TrimSpace(userData.String()); inUserData == 0 && v != "" && v != AWSScrubUserData {
					return errors.New("holds a userData that is not the placeholder")
				}
			}
			if inUserData == 0 {
				userData.Reset()
			}
		}
		if err := checkScrubbedText(text.String()); err != nil {
			return err
		}
		text.Reset()
	}
}

// checkScrubbedJSONBody checks a json body's every key, string and number,
// and refuses a Value (in any case, as a struct decode reads it), in any
// object, that is neither AWSScrubParameterValue nor a public
// parameter's.
func checkScrubbedJSONBody(body string) error {
	if err := (awsJSONCheck{foldKeys: true}).check([]byte(body)); err != nil {
		return err
	}
	var answer any
	if decodeOneJSON([]byte(body), &answer) != nil {
		return errors.New("does not parse as JSON")
	}
	return walkJSONObjects("$", answer, func(path string, object map[string]any) error {
		for key, value := range object {
			if strings.EqualFold(key, "Value") && value != AWSScrubParameterValue && !awsPublicParameter(object["Name"]) {
				return fmt.Errorf("%s.%s: is neither the placeholder nor a public parameter's value", path, key)
			}
		}
		return nil
	})
}

// checkScrubbedStateOutputs refuses a sensitive output whose value is
// neither null nor AWSScrubSensitiveValue.
func checkScrubbedStateOutputs(state any) error {
	root, _ := state.(map[string]any)
	outputs, ok := root["outputs"].(map[string]any)
	if !ok && root["outputs"] != nil {
		return errors.New("$.outputs: is not an object")
	}
	for _, name := range slices.Sorted(maps.Keys(outputs)) {
		output, _ := outputs[name].(map[string]any)
		if awsSensitiveOutput(output) && !awsHiddenStateValue(output["value"]) {
			return fmt.Errorf("$.outputs.%s.value: is a sensitive output's value, not the placeholder", name)
		}
	}
	return nil
}

// checkScrubbedStateValues refuses, anywhere in the state, a user_data or
// user_data_base64 that is not null or a placeholder; and in a resource
// instance, an attribute its sensitive_attributes lists, or a private
// aws_ssm_parameter's value or insecure_value (the provider marks only
// value sensitive), that is neither null nor AWSScrubSensitiveValue. A
// public SSM parameter's value passes; its other attributes do not.
func checkScrubbedStateValues(state any) error {
	err := walkJSONObjects("$", state, func(path string, object map[string]any) error {
		for _, key := range awsStateUserDataKeys {
			if value := object[key]; value != nil && value != AWSScrubUserData && value != AWSScrubSensitiveValue {
				return fmt.Errorf("%s.%s: is neither null nor the placeholder", path, key)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return walkAWSStateInstances(state, func(path, resourceType string, attributes map[string]any, sensitive []string) error {
		for _, name := range sensitive {
			if !awsHiddenStateValue(attributes[name]) && !awsPublicStateValue(resourceType, attributes, name) {
				return fmt.Errorf("%s.attributes.%s: is listed in sensitive_attributes and is not the placeholder", path, name)
			}
		}
		for _, key := range awsSSMValueKeys {
			if resourceType == "aws_ssm_parameter" && !awsHiddenStateValue(attributes[key]) && !awsPublicStateValue(resourceType, attributes, key) {
				return fmt.Errorf("%s.attributes.%s: is a private SSM parameter's value", path, key)
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

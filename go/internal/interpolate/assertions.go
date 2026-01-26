package interpolate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// AssertionResult holds the result of an assertion evaluation
type AssertionResult struct {
	Passed        bool
	Message       string
	ActualValue   string
	ExpectedValue string
}

// Expression pattern: ${var} operator value
var exprPattern = regexp.MustCompile(
	`^\$\{([^}]+)\}\s+` +
		`(==|!=|>=|<=|>|<|contains|matches|exists|not\s+exists|not\s+contains|is|length|iequal|ieq|icontains|startswith|endswith)\s*` +
		`(.*)$`)

// EvaluateAssertion evaluates an assertion expression
// Examples:
// - ${exit_code} == 0
// - ${stdout} contains "success"
// - ${json:$.status} == "ok"
func EvaluateAssertion(expr string, ctx *Context) AssertionResult {
	expr = strings.TrimSpace(expr)

	match := exprPattern.FindStringSubmatch(expr)
	if match == nil {
		return AssertionResult{
			Passed:  false,
			Message: fmt.Sprintf("Invalid expression syntax: %s", expr),
		}
	}

	varName := match[1]
	operator := strings.ToLower(strings.TrimSpace(match[2]))
	expectedRaw := strings.TrimSpace(match[3])

	// Resolve the variable
	actual, _ := ResolveVariable(varName, ctx)

	// Handle operators that don't need an expected value
	if operator == "exists" {
		passed := actual != nil
		msg := "exists"
		if !passed {
			msg = "does not exist"
		}
		return AssertionResult{
			Passed:      passed,
			Message:     msg,
			ActualValue: fmt.Sprintf("%v", actual),
		}
	}

	if operator == "not exists" {
		passed := actual == nil
		msg := "does not exist"
		if !passed {
			msg = "exists"
		}
		return AssertionResult{
			Passed:      passed,
			Message:     msg,
			ActualValue: fmt.Sprintf("%v", actual),
		}
	}

	// Remove quotes if present
	expected := expectedRaw
	if (strings.HasPrefix(expected, "'") && strings.HasSuffix(expected, "'")) ||
		(strings.HasPrefix(expected, "\"") && strings.HasSuffix(expected, "\"")) {
		expected = expected[1 : len(expected)-1]
	}

	// Interpolate variables in expected value
	expected, _ = Interpolate(expected, ctx)

	// Execute operator
	switch operator {
	case "==":
		return evaluateEquals(actual, expected)

	case "!=":
		return evaluateNotEquals(actual, expected)

	case "contains":
		return evaluateContains(actual, expected, false)

	case "not contains":
		return evaluateNotContains(actual, expected)

	case "icontains":
		return evaluateContains(actual, expected, true)

	case "iequal", "ieq":
		return evaluateIEquals(actual, expected)

	case "startswith":
		return evaluateStartsWith(actual, expected)

	case "endswith":
		return evaluateEndsWith(actual, expected)

	case "matches":
		return evaluateMatches(actual, expected)

	case "is":
		return evaluateIs(actual, expected)

	case "length":
		return evaluateLength(actual, expected)

	case ">", "<", ">=", "<=":
		return evaluateComparison(actual, expected, operator)

	default:
		return AssertionResult{
			Passed:  false,
			Message: fmt.Sprintf("Unknown operator: %s", operator),
		}
	}
}

func evaluateEquals(actual any, expected string) AssertionResult {
	actualStr := fmt.Sprintf("%v", actual)

	// Try numeric comparison
	if actualNum, err := toFloat64(actual); err == nil {
		if expectedNum, err := strconv.ParseFloat(expected, 64); err == nil {
			passed := actualNum == expectedNum
			return AssertionResult{
				Passed:        passed,
				Message:       fmt.Sprintf("actual=%v, expected=%v", actualNum, expectedNum),
				ActualValue:   fmt.Sprintf("%v", actualNum),
				ExpectedValue: expected,
			}
		}
	}

	// String comparison
	passed := actualStr == expected
	return AssertionResult{
		Passed:        passed,
		Message:       fmt.Sprintf("actual=%q, expected=%q", actualStr, expected),
		ActualValue:   actualStr,
		ExpectedValue: expected,
	}
}

func evaluateNotEquals(actual any, expected string) AssertionResult {
	actualStr := fmt.Sprintf("%v", actual)

	// Try numeric comparison
	if actualNum, err := toFloat64(actual); err == nil {
		if expectedNum, err := strconv.ParseFloat(expected, 64); err == nil {
			passed := actualNum != expectedNum
			return AssertionResult{
				Passed:        passed,
				Message:       fmt.Sprintf("actual=%v, should not equal %v", actualNum, expectedNum),
				ActualValue:   fmt.Sprintf("%v", actualNum),
				ExpectedValue: expected,
			}
		}
	}

	// String comparison
	passed := actualStr != expected
	return AssertionResult{
		Passed:        passed,
		Message:       fmt.Sprintf("actual=%q, should not equal %q", actualStr, expected),
		ActualValue:   actualStr,
		ExpectedValue: expected,
	}
}

func evaluateContains(actual any, expected string, caseInsensitive bool) AssertionResult {
	actualStr := fmt.Sprintf("%v", actual)
	search := expected

	if caseInsensitive {
		actualStr = strings.ToLower(actualStr)
		search = strings.ToLower(expected)
	}

	passed := strings.Contains(actualStr, search)
	msg := "contains"
	if !passed {
		msg = "does not contain"
	}
	if caseInsensitive {
		msg += " (case-insensitive)"
	}

	return AssertionResult{
		Passed:        passed,
		Message:       fmt.Sprintf("%s %q", msg, expected),
		ActualValue:   fmt.Sprintf("%v", actual),
		ExpectedValue: expected,
	}
}

func evaluateNotContains(actual any, expected string) AssertionResult {
	actualStr := fmt.Sprintf("%v", actual)
	passed := !strings.Contains(actualStr, expected)
	msg := "does not contain"
	if !passed {
		msg = "contains"
	}

	return AssertionResult{
		Passed:        passed,
		Message:       fmt.Sprintf("%s %q", msg, expected),
		ActualValue:   actualStr,
		ExpectedValue: expected,
	}
}

func evaluateIEquals(actual any, expected string) AssertionResult {
	actualStr := strings.TrimSpace(strings.ToLower(fmt.Sprintf("%v", actual)))
	expectedNorm := strings.TrimSpace(strings.ToLower(expected))
	passed := actualStr == expectedNorm

	return AssertionResult{
		Passed:        passed,
		Message:       fmt.Sprintf("actual=%q, expected (case-insensitive)=%q", actual, expected),
		ActualValue:   fmt.Sprintf("%v", actual),
		ExpectedValue: expected,
	}
}

func evaluateStartsWith(actual any, expected string) AssertionResult {
	actualStr := fmt.Sprintf("%v", actual)
	passed := strings.HasPrefix(actualStr, expected)
	msg := "starts with"
	if !passed {
		msg = "does not start with"
	}

	return AssertionResult{
		Passed:        passed,
		Message:       fmt.Sprintf("%s %q", msg, expected),
		ActualValue:   actualStr,
		ExpectedValue: expected,
	}
}

func evaluateEndsWith(actual any, expected string) AssertionResult {
	actualStr := fmt.Sprintf("%v", actual)
	passed := strings.HasSuffix(actualStr, expected)
	msg := "ends with"
	if !passed {
		msg = "does not end with"
	}

	return AssertionResult{
		Passed:        passed,
		Message:       fmt.Sprintf("%s %q", msg, expected),
		ActualValue:   actualStr,
		ExpectedValue: expected,
	}
}

func evaluateMatches(actual any, pattern string) AssertionResult {
	actualStr := fmt.Sprintf("%v", actual)

	re, err := regexp.Compile(pattern)
	if err != nil {
		return AssertionResult{
			Passed:        false,
			Message:       fmt.Sprintf("Invalid regex pattern: %v", err),
			ActualValue:   actualStr,
			ExpectedValue: pattern,
		}
	}

	passed := re.MatchString(actualStr)
	msg := "matches"
	if !passed {
		msg = "does not match"
	}

	return AssertionResult{
		Passed:        passed,
		Message:       fmt.Sprintf("%s pattern %q", msg, pattern),
		ActualValue:   actualStr,
		ExpectedValue: pattern,
	}
}

func evaluateIs(actual any, expectedType string) AssertionResult {
	actualType := getTypeName(actual)
	expectedTypeLower := strings.ToLower(expectedType)

	typeAliases := map[string][]string{
		"string":  {"string", "str"},
		"number":  {"number", "int", "integer", "float"},
		"bool":    {"bool", "boolean"},
		"array":   {"array", "list", "slice"},
		"object":  {"object", "dict", "map"},
		"null":    {"null", "none", "nil"},
	}

	passed := false
	for canonicalType, aliases := range typeAliases {
		for _, alias := range aliases {
			if alias == expectedTypeLower {
				passed = actualType == canonicalType
				break
			}
		}
		if passed {
			break
		}
	}

	return AssertionResult{
		Passed:        passed,
		Message:       fmt.Sprintf("type is %s, expected %s", actualType, expectedType),
		ActualValue:   actualType,
		ExpectedValue: expectedType,
	}
}

func evaluateLength(actual any, expected string) AssertionResult {
	// Parse "length > 5" style
	lengthPattern := regexp.MustCompile(`([><=!]+)\s*(\d+)`)
	match := lengthPattern.FindStringSubmatch(expected)

	if match == nil {
		return AssertionResult{
			Passed:        false,
			Message:       fmt.Sprintf("Invalid length expression: %s", expected),
			ExpectedValue: expected,
		}
	}

	op := match[1]
	lengthVal, _ := strconv.Atoi(match[2])

	actualLen := getLength(actual)

	var passed bool
	switch op {
	case ">":
		passed = actualLen > lengthVal
	case "<":
		passed = actualLen < lengthVal
	case ">=":
		passed = actualLen >= lengthVal
	case "<=":
		passed = actualLen <= lengthVal
	case "==":
		passed = actualLen == lengthVal
	case "!=":
		passed = actualLen != lengthVal
	default:
		return AssertionResult{
			Passed:        false,
			Message:       fmt.Sprintf("Unknown length operator: %s", op),
			ExpectedValue: expected,
		}
	}

	return AssertionResult{
		Passed:        passed,
		Message:       fmt.Sprintf("length=%d, expected %s %d", actualLen, op, lengthVal),
		ActualValue:   strconv.Itoa(actualLen),
		ExpectedValue: fmt.Sprintf("%s %d", op, lengthVal),
	}
}

func evaluateComparison(actual any, expected string, operator string) AssertionResult {
	actualNum, err := toFloat64(actual)
	if err != nil {
		return AssertionResult{
			Passed:        false,
			Message:       fmt.Sprintf("Cannot compare: %v %s %s", actual, operator, expected),
			ActualValue:   fmt.Sprintf("%v", actual),
			ExpectedValue: expected,
		}
	}

	expectedNum, err := strconv.ParseFloat(expected, 64)
	if err != nil {
		return AssertionResult{
			Passed:        false,
			Message:       fmt.Sprintf("Cannot compare: %v %s %s", actual, operator, expected),
			ActualValue:   fmt.Sprintf("%v", actual),
			ExpectedValue: expected,
		}
	}

	var passed bool
	switch operator {
	case ">":
		passed = actualNum > expectedNum
	case "<":
		passed = actualNum < expectedNum
	case ">=":
		passed = actualNum >= expectedNum
	case "<=":
		passed = actualNum <= expectedNum
	}

	return AssertionResult{
		Passed:        passed,
		Message:       fmt.Sprintf("actual=%v, expected %s %v", actualNum, operator, expectedNum),
		ActualValue:   fmt.Sprintf("%v", actualNum),
		ExpectedValue: fmt.Sprintf("%s %v", operator, expectedNum),
	}
}

func toFloat64(v any) (float64, error) {
	switch val := v.(type) {
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case float64:
		return val, nil
	case string:
		return strconv.ParseFloat(val, 64)
	default:
		return 0, fmt.Errorf("cannot convert to float64: %T", v)
	}
}

func getTypeName(v any) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case string:
		return "string"
	case int, int64, float64:
		return "number"
	case bool:
		return "bool"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

func getLength(v any) int {
	switch val := v.(type) {
	case string:
		return len(val)
	case []any:
		return len(val)
	case map[string]any:
		return len(val)
	default:
		return 0
	}
}

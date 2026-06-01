package validator

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	alphaNumericRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	emailRegex       = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	idRegex          = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	safePathRegex    = regexp.MustCompile(`^[a-zA-Z0-9/_-]+$`)
)

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

type StringValidator struct {
	value    string
	field    string
	errors   ValidationErrors
	required bool
}

func ValidateString(value string, field string) *StringValidator {
	return &StringValidator{
		value:  value,
		field:  field,
		errors: make(ValidationErrors, 0),
	}
}

func (v *StringValidator) Required() *StringValidator {
	v.required = true
	if v.value == "" {
		v.errors = append(v.errors, ValidationError{Field: v.field, Message: "不能为空"})
	}
	return v
}

func (v *StringValidator) MinLength(min int) *StringValidator {
	if v.value != "" && len(v.value) < min {
		v.errors = append(v.errors, ValidationError{
			Field:   v.field,
			Message: fmt.Sprintf("长度不能少于 %d 个字符", min),
		})
	}
	return v
}

func (v *StringValidator) MaxLength(max int) *StringValidator {
	if v.value != "" && len(v.value) > max {
		v.errors = append(v.errors, ValidationError{
			Field:   v.field,
			Message: fmt.Sprintf("长度不能超过 %d 个字符", max),
		})
	}
	return v
}

func (v *StringValidator) AlphaNumeric() *StringValidator {
	if v.value != "" && !alphaNumericRegex.MatchString(v.value) {
		v.errors = append(v.errors, ValidationError{
			Field:   v.field,
			Message: "只能包含字母、数字、下划线和连字符",
		})
	}
	return v
}

func (v *StringValidator) Email() *StringValidator {
	if v.value != "" && !emailRegex.MatchString(v.value) {
		v.errors = append(v.errors, ValidationError{
			Field:   v.field,
			Message: "无效的邮箱格式",
		})
	}
	return v
}

func (v *StringValidator) MatchRegex(pattern *regexp.Regexp) *StringValidator {
	if v.value != "" && !pattern.MatchString(v.value) {
		v.errors = append(v.errors, ValidationError{
			Field:   v.field,
			Message: "格式不符合要求",
		})
	}
	return v
}

func (v *StringValidator) InList(allowed ...string) *StringValidator {
	if v.value == "" {
		return v
	}
	for _, s := range allowed {
		if v.value == s {
			return v
		}
	}
	v.errors = append(v.errors, ValidationError{
		Field:   v.field,
		Message: fmt.Sprintf("值必须是以下之一: %s", strings.Join(allowed, ", ")),
	})
	return v
}

func (v *StringValidator) NotInList(forbidden ...string) *StringValidator {
	if v.value == "" {
		return v
	}
	for _, s := range forbidden {
		if v.value == s {
			v.errors = append(v.errors, ValidationError{
				Field:   v.field,
				Message: fmt.Sprintf("值不能是: %s", s),
			})
			return v
		}
	}
	return v
}

func (v *StringValidator) Error() error {
	if v.errors.HasErrors() {
		return v.errors
	}
	return nil
}

type IntValidator struct {
	value  int
	field  string
	errors ValidationErrors
}

func ValidateInt(value int, field string) *IntValidator {
	return &IntValidator{
		value:  value,
		field:  field,
		errors: make(ValidationErrors, 0),
	}
}

func (v *IntValidator) Min(min int) *IntValidator {
	if v.value < min {
		v.errors = append(v.errors, ValidationError{
			Field:   v.field,
			Message: fmt.Sprintf("值不能小于 %d", min),
		})
	}
	return v
}

func (v *IntValidator) Max(max int) *IntValidator {
	if v.value > max {
		v.errors = append(v.errors, ValidationError{
			Field:   v.field,
			Message: fmt.Sprintf("值不能大于 %d", max),
		})
	}
	return v
}

func (v *IntValidator) InRange(min, max int) *IntValidator {
	return v.Min(min).Max(max)
}

func (v *IntValidator) Error() error {
	if v.errors.HasErrors() {
		return v.errors
	}
	return nil
}

type ArrayValidator struct {
	value  []string
	field  string
	errors ValidationErrors
}

func ValidateArray(value []string, field string) *ArrayValidator {
	return &ArrayValidator{
		value:  value,
		field:  field,
		errors: make(ValidationErrors, 0),
	}
}

func (v *ArrayValidator) NotEmpty() *ArrayValidator {
	if len(v.value) == 0 {
		v.errors = append(v.errors, ValidationError{
			Field:   v.field,
			Message: "不能为空",
		})
	}
	return v
}

func (v *ArrayValidator) MaxLength(max int) *ArrayValidator {
	if len(v.value) > max {
		v.errors = append(v.errors, ValidationError{
			Field:   v.field,
			Message: fmt.Sprintf("长度不能超过 %d", max),
		})
	}
	return v
}

func (v *ArrayValidator) Each(fn func(value string) error) *ArrayValidator {
	for _, item := range v.value {
		if err := fn(item); err != nil {
			v.errors = append(v.errors, ValidationError{
				Field:   v.field,
				Message: err.Error(),
			})
		}
	}
	return v
}

func (v *ArrayValidator) Error() error {
	if v.errors.HasErrors() {
		return v.errors
	}
	return nil
}

func ValidateID(id string) error {
	if id == "" {
		return ValidationError{Field: "id", Message: "ID 不能为空"}
	}
	if len(id) > 64 {
		return ValidationError{Field: "id", Message: "ID 长度不能超过 64 个字符"}
	}
	if !idRegex.MatchString(id) {
		return ValidationError{Field: "id", Message: "ID 只能包含字母、数字、下划线和连字符"}
	}
	return nil
}

func ValidateUsername(username string) error {
	if username == "" {
		return ValidationError{Field: "username", Message: "用户名不能为空"}
	}
	if len(username) < 3 {
		return ValidationError{Field: "username", Message: "用户名长度不能少于 3 个字符"}
	}
	if len(username) > 50 {
		return ValidationError{Field: "username", Message: "用户名长度不能超过 50 个字符"}
	}
	if !alphaNumericRegex.MatchString(username) {
		return ValidationError{Field: "username", Message: "用户名只能包含字母和数字"}
	}
	return nil
}

func ValidatePassword(password string) error {
	if password == "" {
		return ValidationError{Field: "password", Message: "密码不能为空"}
	}
	if len(password) < 8 {
		return ValidationError{Field: "password", Message: "密码长度不能少于 8 个字符"}
	}
	if len(password) > 128 {
		return ValidationError{Field: "password", Message: "密码长度不能超过 128 个字符"}
	}
	return nil
}

func ValidateRole(role string) error {
	return ValidateString(role, "role").
		Required().
		InList("admin", "editor", "viewer").
		Error()
}

func ValidateDateFormat(date string) error {
	if date == "" {
		return ValidationError{Field: "date", Message: "日期不能为空"}
	}
	dateRegex := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	if !dateRegex.MatchString(date) {
		return ValidationError{Field: "date", Message: "日期格式必须是 YYYY-MM-DD"}
	}
	return nil
}

func ValidateProbeID(probeID string) error {
	return ValidateString(probeID, "probe_id").
		MaxLength(64).
		AlphaNumeric().
		Error()
}

func SanitizeSQLString(input string) string {
	replacer := strings.NewReplacer(
		"'", "''",
		"\\", "\\\\",
	)
	return replacer.Replace(input)
}

func IsSafeSQLIdentifier(identifier string) bool {
	if identifier == "" {
		return false
	}
	safeRegex := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	return safeRegex.MatchString(identifier)
}

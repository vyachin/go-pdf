package pdfgen

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/engine"
)

// The public types are mirrors of the internal ones. These tests fail when the
// declarations drift apart or when a conversion stops carrying a field. They are
// reflected rather than listed so that a new field enters the checks without
// anybody remembering to add it.

// fieldShape is one field of a mirrored struct: its name and the name of its
// type. The two declarations are deliberately separate types, so a type is
// compared by name (Limits against Limits, time.Time against time.Time) rather
// than by package-qualified identity.
type fieldShape struct {
	name string
	typ  string
}

var timeType = reflect.TypeOf(time.Time{})

func isTime(t reflect.Type) bool { return t == timeType }

// shapeOf returns the exported fields of t in declaration order. A field hidden
// from callers is drift: both sides of the mirror export every field.
func shapeOf(t reflect.Type) ([]fieldShape, error) {
	fields := make([]fieldShape, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			return nil, fmt.Errorf("%s.%s is unexported; the mirrored structs export every field", t, field.Name)
		}
		fields = append(fields, fieldShape{field.Name, field.Type.Name()})
	}
	return fields, nil
}

// posMirror maps each diag.Position field to the public Diagnostic field that
// mirrors it: the internal position is flattened in the public type.
var posMirror = map[string]string{"File": "File", "Line": "Line", "Col": "Column"}

// diagnosticShape returns one shape for both diagnostics, flattening
// diag.Position into File, Line and Column so the internal fields line up with
// the public ones. A field added to diag.Position, or to either diagnostic, has
// no counterpart and is reported by compareShapes.
func diagnosticShape(t reflect.Type, internal bool) ([]fieldShape, error) {
	var fields []fieldShape
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			return nil, fmt.Errorf("%s.%s is unexported; the mirrored structs export every field", t, field.Name)
		}
		if !internal || field.Name != "Pos" {
			fields = append(fields, fieldShape{field.Name, field.Type.Name()})
			continue
		}
		for j := 0; j < field.Type.NumField(); j++ {
			pos := field.Type.Field(j)
			if !pos.IsExported() {
				return nil, fmt.Errorf("%s.%s.%s is unexported; the mirrored structs export every field", t, field.Name, pos.Name)
			}
			name, ok := posMirror[pos.Name]
			if !ok {
				return nil, fmt.Errorf("%s.%s.%s has no public mirror", t, field.Name, pos.Name)
			}
			fields = append(fields, fieldShape{name, pos.Type.Name()})
		}
	}
	return fields, nil
}

// plainShape adapts shapeOf to the table in TestPublicTypesMirrorInternals.
func plainShape(t reflect.Type, _ bool) ([]fieldShape, error) { return shapeOf(t) }

// compareShapes reports every field present on one side only, every type-name
// mismatch and a differing field count, naming the field and the type name.
// Declaration order is not part of the contract: the structs are independent
// declarations, so the mirror guarantees the name/type set and its size.
func compareShapes(public, internal []fieldShape) error {
	var problems []string
	if len(public) != len(internal) {
		problems = append(problems, fmt.Sprintf("field count: %d public, %d internal", len(public), len(internal)))
	}
	pub := make(map[string]string, len(public))
	for _, field := range public {
		pub[field.name] = field.typ
	}
	inn := make(map[string]string, len(internal))
	for _, field := range internal {
		inn[field.name] = field.typ
	}
	names := make([]string, 0, len(pub)+len(inn))
	for name := range pub {
		names = append(names, name)
	}
	for name := range inn {
		if _, ok := pub[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		publicType, inPublic := pub[name]
		internalType, inInternal := inn[name]
		switch {
		case !inInternal:
			problems = append(problems, fmt.Sprintf("field %s (%s) has no internal counterpart", name, publicType))
		case !inPublic:
			problems = append(problems, fmt.Sprintf("field %s (%s) has no public counterpart", name, internalType))
		case publicType != internalType:
			problems = append(problems, fmt.Sprintf("field %s is %s publicly but %s internally", name, publicType, internalType))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}

// TestPublicTypesMirrorInternals fails when a field is added to or removed from
// one side of a public/internal pair, when a type name drifts, or when a field
// becomes unexported. Diagnostic.Severity is part of the comparison; the round
// trip test proves that both of its values reach the public type.
func TestPublicTypesMirrorInternals(t *testing.T) {
	cases := []struct {
		name     string
		public   reflect.Type
		internal reflect.Type
		shape    func(reflect.Type, bool) ([]fieldShape, error)
	}{
		{"Options", reflect.TypeOf(Options{}), reflect.TypeOf(engine.Options{}), plainShape},
		{"Limits", reflect.TypeOf(Limits{}), reflect.TypeOf(engine.Limits{}), plainShape},
		{"Diagnostic", reflect.TypeOf(Diagnostic{}), reflect.TypeOf(diag.Diagnostic{}), diagnosticShape},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			public, err := c.shape(c.public, false)
			if err != nil {
				t.Fatalf("public %s: %v", c.name, err)
			}
			internal, err := c.shape(c.internal, true)
			if err != nil {
				t.Fatalf("internal %s: %v", c.name, err)
			}
			if err := compareShapes(public, internal); err != nil {
				t.Fatalf("%s drifts from its internal counterpart: %v", c.name, err)
			}
		})
	}
}

// TestMirrorShapesFailOnDrift mutates copies of the real field sets and requires
// the comparison to fail with the field and the type. Without it the mirror test
// above could pass vacuously.
func TestMirrorShapesFailOnDrift(t *testing.T) {
	public, err := shapeOf(reflect.TypeOf(Limits{}))
	if err != nil {
		t.Fatal(err)
	}
	internal, err := shapeOf(reflect.TypeOf(engine.Limits{}))
	if err != nil {
		t.Fatal(err)
	}

	// A field added on the public side only.
	added := append(append([]fieldShape(nil), public...), fieldShape{"MaxBytes", "int"})
	if err := compareShapes(added, internal); err == nil || !strings.Contains(err.Error(), "MaxBytes") || !strings.Contains(err.Error(), "int") {
		t.Fatalf("an added field was not reported by name and type: %v", err)
	}
	// A field removed from the public side only.
	dropped := append([]fieldShape(nil), public[1:]...)
	if err := compareShapes(dropped, internal); err == nil || !strings.Contains(err.Error(), public[0].name) {
		t.Fatalf("a removed field was not reported: %v", err)
	}
	// A type that drifted on the internal side.
	retyped := append([]fieldShape(nil), internal...)
	retyped[0].typ = "int64"
	if err := compareShapes(public, retyped); err == nil || !strings.Contains(err.Error(), retyped[0].name) || !strings.Contains(err.Error(), "int64") {
		t.Fatalf("a retyped field was not reported by name and type: %v", err)
	}

	// A field hidden from callers is drift too. reflect.StructOf builds the
	// mutated copy without adding an unused struct to the package.
	hidden := reflect.StructOf([]reflect.StructField{
		{Name: "Name", Type: reflect.TypeOf("")},
		{Name: "count", Type: reflect.TypeOf(0), PkgPath: "github.com/vyachin/go-pdf/pdfgen"},
	})
	if _, err := shapeOf(hidden); err == nil || !strings.Contains(err.Error(), "count") {
		t.Fatalf("an unexported field was not reported: %v", err)
	}

	// diag.Position is flattened into three public fields; a field added to it
	// has no public mirror and must be reported.
	position := reflect.StructOf([]reflect.StructField{
		{Name: "File", Type: reflect.TypeOf("")},
		{Name: "Line", Type: reflect.TypeOf(0)},
		{Name: "Col", Type: reflect.TypeOf(0)},
		{Name: "Offset", Type: reflect.TypeOf(0)},
	})
	synthetic := reflect.StructOf([]reflect.StructField{
		{Name: "Severity", Type: reflect.TypeOf(diag.Error)},
		{Name: "Code", Type: reflect.TypeOf("")},
		{Name: "Message", Type: reflect.TypeOf("")},
		{Name: "Pos", Type: position},
		{Name: "Element", Type: reflect.TypeOf("")},
	})
	if _, err := diagnosticShape(synthetic, true); err == nil || !strings.Contains(err.Error(), "Offset") {
		t.Fatalf("a diag.Position field without a public mirror was not reported: %v", err)
	}
}

// fillMarkers sets every field to a value derived from the field kind and a
// running counter: ints count up, strings are "v<n>", bools are true and a
// time.Time gets one fixed instant. A zero value in a converted field therefore
// means the field was not carried.
func fillMarkers(value reflect.Value, next *int) {
	switch value.Kind() {
	case reflect.Int:
		*next = *next + 1
		value.SetInt(int64(*next))
	case reflect.String:
		*next = *next + 1
		value.SetString(fmt.Sprintf("v%d", *next))
	case reflect.Bool:
		value.SetBool(true)
	case reflect.Struct:
		if isTime(value.Type()) {
			value.Set(reflect.ValueOf(time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)))
			return
		}
		for i := 0; i < value.NumField(); i++ {
			fillMarkers(value.Field(i), next)
		}
	}
}

// compareMirrorValues requires every field of source to be carried by the
// converted value and matches the two structs by field name. It reports every
// difference, naming the field path and the conversion, and recurses into a
// mirrored struct so that a nested drift is named as Limits.MaxPages.
func compareMirrorValues(converted, source reflect.Value, direction string) error {
	var problems []string
	walkMirrorValues(converted, source, source.Type().Name(), direction, &problems)
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %s", direction, strings.Join(problems, "; "))
}

func walkMirrorValues(converted, source reflect.Value, path, direction string, problems *[]string) {
	if source.Kind() == reflect.Struct && !isTime(source.Type()) {
		if converted.Kind() != reflect.Struct || isTime(converted.Type()) {
			*problems = append(*problems, fmt.Sprintf("%s is not a struct in %s", path, direction))
			return
		}
		for i := 0; i < source.NumField(); i++ {
			field := source.Type().Field(i)
			carried := converted.FieldByName(field.Name)
			if !carried.IsValid() {
				*problems = append(*problems, fmt.Sprintf("%s.%s (%s) is not carried by %s", path, field.Name, field.Type.Name(), direction))
				continue
			}
			walkMirrorValues(carried, source.Field(i), path+"."+field.Name, direction, problems)
		}
		for i := 0; i < converted.NumField(); i++ {
			name := converted.Type().Field(i).Name
			if _, ok := source.Type().FieldByName(name); !ok {
				*problems = append(*problems, fmt.Sprintf("%s.%s has no source field in %s", path, name, direction))
			}
		}
		return
	}
	if !reflect.DeepEqual(converted.Interface(), source.Interface()) {
		*problems = append(*problems, fmt.Sprintf("%s is %v in %s, want %v", path, converted.Interface(), direction, source.Interface()))
	}
}

// TestOptionsReachTheEngine sets every public field to a unique marker and
// requires the converted engine value to carry it. A field dropped from toEngine
// stays zero and is named. Options has no reverse conversion by design, so this
// direction is the whole public-to-engine contract.
func TestOptionsReachTheEngine(t *testing.T) {
	var opts Options
	fillMarkers(reflect.ValueOf(&opts).Elem(), new(int))
	converted := opts.toEngine()
	if err := compareMirrorValues(reflect.ValueOf(converted), reflect.ValueOf(opts), "pdfgen.Options.toEngine"); err != nil {
		t.Fatal(err)
	}

	// The assertion must be able to fail: clearing one converted field models a
	// field that toEngine forgot to carry.
	broken := converted
	broken.Charset = ""
	if err := compareMirrorValues(reflect.ValueOf(broken), reflect.ValueOf(opts), "pdfgen.Options.toEngine"); err == nil || !strings.Contains(err.Error(), "Options.Charset") {
		t.Fatalf("a dropped Options field was not named: %v", err)
	}
}

// TestLimitsAndDiagnosticsRoundTrip checks both Limits conversions and the
// diagnostic conversion. Every field is set to a marker; a mapping that drops a
// field leaves the destination at zero and the comparison names the field and
// the direction.
func TestLimitsAndDiagnosticsRoundTrip(t *testing.T) {
	t.Run("Limits", func(t *testing.T) {
		var limits Limits
		fillMarkers(reflect.ValueOf(&limits).Elem(), new(int))
		internal := limits.toEngine()
		if err := compareMirrorValues(reflect.ValueOf(internal), reflect.ValueOf(limits), "pdfgen.Limits.toEngine"); err != nil {
			t.Fatal(err)
		}
		back := fromEngineLimits(internal)
		if err := compareMirrorValues(reflect.ValueOf(back), reflect.ValueOf(internal), "pdfgen.fromEngineLimits"); err != nil {
			t.Fatal(err)
		}

		brokenInternal := internal
		brokenInternal.MaxPages = 0
		if err := compareMirrorValues(reflect.ValueOf(brokenInternal), reflect.ValueOf(limits), "pdfgen.Limits.toEngine"); err == nil || !strings.Contains(err.Error(), "Limits.MaxPages") {
			t.Fatalf("a Limits field dropped towards the engine was not named: %v", err)
		}
		brokenBack := back
		brokenBack.MaxPages = 0
		if err := compareMirrorValues(reflect.ValueOf(brokenBack), reflect.ValueOf(internal), "pdfgen.fromEngineLimits"); err == nil || !strings.Contains(err.Error(), "Limits.MaxPages") {
			t.Fatalf("a Limits field dropped from the engine was not named: %v", err)
		}
	})

	t.Run("Diagnostics", func(t *testing.T) {
		cases := []struct {
			name     string
			internal diag.Diagnostic
			want     Diagnostic
		}{
			{
				name: "warning",
				internal: diag.Diagnostic{
					Severity: diag.Warning,
					Code:     "css/unsupported-value",
					Message:  "unsupported value",
					Pos:      diag.Position{File: "sheet.css", Line: 7, Col: 12},
					Element:  "body > p",
				},
				want: Diagnostic{
					Severity: SeverityWarning,
					Code:     "css/unsupported-value",
					Message:  "unsupported value",
					File:     "sheet.css",
					Line:     7,
					Column:   12,
					Element:  "body > p",
				},
			},
			{
				name: "error",
				internal: diag.Diagnostic{
					Severity: diag.Error,
					Code:     "font/glyph-missing",
					Message:  "missing glyph",
					Pos:      diag.Position{File: "invoice.html", Line: 3, Col: 1},
				},
				want: Diagnostic{
					Severity: SeverityError,
					Code:     "font/glyph-missing",
					Message:  "missing glyph",
					File:     "invoice.html",
					Line:     3,
					Column:   1,
				},
			},
		}
		for _, c := range cases {
			got := fromDiags(diag.List{c.internal}, "document.html")
			if len(got) != 1 {
				t.Fatalf("%s: fromDiags returned %d diagnostics, want 1", c.name, len(got))
			}
			if err := compareMirrorValues(reflect.ValueOf(got[0]), reflect.ValueOf(c.want), "fromDiags (diag.List -> pdfgen.Diagnostics)"); err != nil {
				t.Fatal(err)
			}

			broken := got[0]
			broken.Element = "wrong"
			if err := compareMirrorValues(reflect.ValueOf(broken), reflect.ValueOf(c.want), "fromDiags (diag.List -> pdfgen.Diagnostics)"); err == nil || !strings.Contains(err.Error(), "Diagnostic.Element") {
				t.Fatalf("%s: a dropped diagnostic field was not named: %v", c.name, err)
			}
		}
	})
}

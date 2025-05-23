package claude

import (
	"fmt"
)

// JSONSchemaType represents the possible types in a JSON Schema.
type JSONSchemaType string

const (
	ObjectType  JSONSchemaType = "object"
	ArrayType   JSONSchemaType = "array"
	StringType  JSONSchemaType = "string"
	NumberType  JSONSchemaType = "number"  // Represents both integer and float
	IntegerType JSONSchemaType = "integer" // More specific than number
	BooleanType JSONSchemaType = "boolean"
	NullType    JSONSchemaType = "null"
)

// JSONSchema defines the structure for a JSON schema, as used by Anthropic.
type JSONSchema struct {
	// Type specifies the data type for the schema.
	// Required.
	Type JSONSchemaType `json:"type"`

	Description string `json:"description,omitempty"`

	// Properties defines the properties of an object if Type is "object".
	// Keys are property names, values are nested JSONSchema definitions.
	// Optional, but required if Type is "object" and you want to define properties.
	Properties map[string]*JSONSchema `json:"properties,omitempty"`

	// Required lists the names of properties that must be present if Type is "object".
	// Optional.
	Required []string `json:"required,omitempty"`

	// Items defines the schema for elements in an array if Type is "array".
	// Optional, but required if Type is "array" and you want to define item structure.
	Items *JSONSchema `json:"items,omitempty"`

	// Enum lists the set of allowed values for a schema.
	// The values can be of any type, but usually strings or numbers for tool inputs.
	// Optional.
	Enum []any `json:"enum,omitempty"`

	// Format provides additional semantic meaning to a string type (e.g., "date-time", "email").
	// Optional.
	Format string `json:"format,omitempty"`

	// Minimum specifies the minimum numeric value.
	// Optional, for "number" or "integer" types.
	Minimum *float64 `json:"minimum,omitempty"` // Use pointer to distinguish 0 from not set

	// Maximum specifies the maximum numeric value.
	// Optional, for "number" or "integer" types.
	Maximum *float64 `json:"maximum,omitempty"` // Use pointer to distinguish 0 from not set

	// MinLength specifies the minimum string length.
	// Optional, for "string" type.
	MinLength *int `json:"minLength,omitempty"` // Use pointer to distinguish 0 from not set

	// MaxLength specifies the maximum string length.
	// Optional, for "string" type.
	MaxLength *int `json:"maxLength,omitempty"` // Use pointer to distinguish 0 from not set

	// Pattern specifies a regex pattern for string validation.
	// Optional, for "string" type.
	Pattern string `json:"pattern,omitempty"`

	// Default provides a default value for a property.
	// Optional.
	Default any `json:"default,omitempty"`

	// Examples provides example values.
	// Optional.
	Examples []any `json:"examples,omitempty"`
}

//nolint:unused
func examples() {
	// Example: Schema for a weather tool
	weatherToolSchema := &JSONSchema{
		Type:        ObjectType,
		Description: "Schema for getting the current weather in a location.",
		Properties: map[string]*JSONSchema{
			"location": {
				Type:        StringType,
				Description: "The city and state, e.g. San Francisco, CA",
			},
			"unit": {
				Type:        StringType,
				Description: "The unit of temperature, either 'celsius' or 'fahrenheit'",
				Enum:        []interface{}{"celsius", "fahrenheit"},
				Default:     "celsius",
			},
			"days_forecast": {
				Type:        IntegerType,
				Description: "Number of days to forecast, between 1 and 7 inclusive.",
				Minimum:     floatPtr(1), // Helper function for pointer to float64
				Maximum:     floatPtr(7),
			},
		},
		Required: []string{"location"},
	}

	shoppingListSchema := &JSONSchema{
		Type:        ObjectType,
		Description: "Schema for a shopping list tool.",
		Properties: map[string]*JSONSchema{
			"list_name": {
				Type:        StringType,
				Description: "The name of the shopping list.",
			},
			"items": {
				Type:        ArrayType,
				Description: "A list of items to add to the shopping list.",
				Items: &JSONSchema{
					Type: ObjectType,
					Properties: map[string]*JSONSchema{
						"name": {
							Type:        StringType,
							Description: "Name of the item.",
						},
						"quantity": {
							Type:        IntegerType,
							Description: "Quantity of the item.",
							Minimum:     floatPtr(1),
							Default:     1,
						},
					},
					Required: []string{"name"},
				},
			},
		},
		Required: []string{"list_name", "items"},
	}

	fmt.Println(weatherToolSchema, shoppingListSchema)
}

// Helper function to get a pointer to a float64, useful for Minimum/Maximum
func floatPtr(f float64) *float64 {
	return &f
}

// Helper function to get a pointer to an int, useful for MinLength/MaxLength
func intPtr(i int) *int {
	return &i
}

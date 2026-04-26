package middleware

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// methodCallTool is the JSON-RPC method name for tool calls.
const methodCallTool = "tools/call"

// BoolFieldRegistry maps tool names to the set of JSON parameter names that
// were declared as boolean (or *bool) on the corresponding Go Params struct.
type BoolFieldRegistry map[string]map[string]struct{}

// NewBoolCoercer returns a receiving middleware that rewrites the literal
// JSON strings "true"/"false" into JSON booleans for the given tool's
// boolean-typed parameters. It compensates for MCP clients that serialize
// booleans as strings on the wire — the SDK validator would otherwise reject
// the call with `type: true has type "string", want one of "null, boolean"`.
//
// The middleware is intentionally narrow:
//
//   - it acts only on method "tools/call";
//   - it acts only on parameters whose JSON name is in registry[toolName];
//   - it rewrites only the exact tokens "true" and "false" — anything else
//     (including null, integers, garbled strings) is left alone so the SDK
//     still surfaces a real type error to the caller.
func NewBoolCoercer(registry BoolFieldRegistry) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != methodCallTool {
				return next(ctx, method, req)
			}

			ctReq, ok := req.(*mcp.CallToolRequest)
			if !ok || ctReq.Params == nil {
				return next(ctx, method, req)
			}

			fields, ok := registry[ctReq.Params.Name]
			if !ok || len(fields) == 0 {
				return next(ctx, method, req)
			}

			coerced, changed := coerceBoolArgs(ctReq.Params.Arguments, fields)
			if changed {
				ctReq.Params.Arguments = coerced
			}

			return next(ctx, method, req)
		}
	}
}

// coerceBoolArgs walks a JSON object and rewrites string-encoded booleans
// for the listed field names into JSON booleans. Returns the rewritten
// payload and whether any change happened.
func coerceBoolArgs(raw json.RawMessage, fields map[string]struct{}) (json.RawMessage, bool) {
	if len(raw) == 0 || len(fields) == 0 {
		return nil, false
	}

	var args map[string]json.RawMessage

	err := json.Unmarshal(raw, &args)
	if err != nil {
		return nil, false
	}

	changed := false

	for name := range fields {
		val, ok := args[name]
		if !ok {
			continue
		}

		coerced, didCoerce := coerceBoolToken(val)
		if didCoerce {
			args[name] = coerced
			changed = true
		}
	}

	if !changed {
		return nil, false
	}

	out, err := json.Marshal(args)
	if err != nil {
		return nil, false
	}

	return out, true
}

// coerceBoolToken rewrites the literal JSON tokens `"true"` / `"false"` into
// `true` / `false`. Any other input (including `null`, numbers, real booleans,
// or strings other than the two listed) returns (_, false).
func coerceBoolToken(raw json.RawMessage) (json.RawMessage, bool) {
	switch string(raw) {
	case `"true"`:
		return json.RawMessage(`true`), true
	case `"false"`:
		return json.RawMessage(`false`), true
	}

	return nil, false
}

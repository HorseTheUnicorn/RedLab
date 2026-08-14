# ADR 0003: Safe declarative rule DSL

Status: accepted

Scenario YAML contains only typed, schema-validated condition and effect objects. It cannot contain shell commands, templates with function calls, JavaScript, Lua, or arbitrary expressions. New behavior is reviewed Go code registered under a versioned name.

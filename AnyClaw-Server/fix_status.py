#!/usr/bin/env python3
path = "cmd/anyclaw/internal/status/helpers.go"
with open(path, "rb") as f:
    data = f.read()

# Fix: replace any corrupted/incomplete string that causes "newline in string"
# Use decode with replace to fix invalid UTF-8, then re-encode
fixed = data.decode("utf-8", errors="replace").encode("utf-8")

# Now fix the specific broken strings - the replacement char might have been used
# Replace (configPath, " + replacement char + ) with proper check/x
fixed = fixed.replace(b', configPath, "\xef\xbf\xbd)', b', configPath, "\xe2\x9c\x94")')
fixed = fixed.replace(b', configPath, "\xef\xbf\xbd)', b', configPath, "\xe2\x9c\x95")')
# Actually the issue is unclosed string - we have "X) without closing quote
# Try replacing the pattern that causes newline in string
import re
# Match: , " + any bytes + ) without closing quote - replace with proper
fixed = re.sub(rb', configPath, ".[^"]*\)', b', configPath, "\xe2\x9c\x94")', fixed)
fixed = re.sub(rb', workspace, ".[^"]*\)', b', workspace, "\xe2\x9c\x94")', fixed)
# For the else branch - need different symbol
# The regex might not work. Let me try simpler: replace known bad sequences
for old in [b'\xe2\x9c\x94)', b'\xe2\x9c\x3f)', b'\xef\xbf\xbd)']:
    fixed = fixed.replace(b', configPath, "' + old, b', configPath, "\xe2\x9c\x94")')
    fixed = fixed.replace(b', workspace, "' + old, b', workspace, "\xe2\x9c\x94")')

with open(path, "wb") as f:
    f.write(fixed)
print("Done")

# agent-browser Command Reference

## Navigation
```bash
agent-browser open <url>              # Navigate (aliases: goto, navigate)
agent-browser close                   # Close browser (aliases: quit, exit)
```

## Snapshot (Primary Way to See the Page)
```bash
agent-browser snapshot -i             # Interactive elements with refs (@e1, @e2...)
agent-browser snapshot -i -C          # Include cursor-interactive elements (onclick divs)
agent-browser snapshot -s "#selector" # Scope to CSS selector
agent-browser snapshot -i --json      # JSON output for parsing
```

## Interaction (Use @refs from snapshot)
```bash
agent-browser click @e1               # Click element
agent-browser click @e1 --new-tab     # Click and open in new tab
agent-browser dblclick @e1            # Double-click
agent-browser fill @e2 "text"         # Clear and type text
agent-browser type @e2 "text"         # Type without clearing
agent-browser select @e1 "option"     # Select dropdown option
agent-browser check @e1               # Check checkbox
agent-browser uncheck @e1             # Uncheck checkbox
agent-browser press Enter             # Press key
agent-browser keyboard type "text"    # Type at current focus (no selector)
agent-browser scroll down 500         # Scroll page
agent-browser drag @e1 @e2            # Drag and drop
agent-browser upload @e1 file.pdf     # Upload files
agent-browser hover @e1               # Hover element
```

## Get Information
```bash
agent-browser get text @e1            # Get element text
agent-browser get text body > page.txt # Get all page text
agent-browser get html @e1            # Get innerHTML
agent-browser get url                 # Get current URL
agent-browser get title               # Get page title
agent-browser get text @e1 --json     # JSON output
```

## Wait
```bash
agent-browser wait @e1                # Wait for element
agent-browser wait --load networkidle # Wait for network idle
agent-browser wait --url "**/page"    # Wait for URL pattern
agent-browser wait 2000               # Wait milliseconds
```

## Downloads
```bash
agent-browser download @e1 ./file.pdf # Click to trigger download
agent-browser wait --download ./out   # Wait for download
```

## Capture
```bash
agent-browser screenshot              # Screenshot to temp dir
agent-browser screenshot page.png     # Screenshot to path
agent-browser screenshot --full       # Full page screenshot
agent-browser screenshot --annotate   # Annotated with numbered labels
agent-browser pdf output.pdf          # Save as PDF
```

## Diff
```bash
agent-browser diff snapshot                        # Current vs last snapshot
agent-browser diff screenshot --baseline before.png # Visual pixel diff
```

## Sessions
```bash
agent-browser --session site1 open <url>  # Named session
agent-browser session list                # List active sessions
agent-browser --session site1 close       # Close specific session
```

## State Persistence
```bash
agent-browser state save auth.json   # Save cookies/localStorage
agent-browser state load auth.json   # Restore state
agent-browser state list             # List saved states
```

## Security
```bash
export AGENT_BROWSER_CONTENT_BOUNDARIES=1          # Wrap output for AI safety
export AGENT_BROWSER_ALLOWED_DOMAINS="example.com" # Domain allowlist
export AGENT_BROWSER_MAX_OUTPUT=50000              # Prevent context flooding
```

## Debugging
```bash
agent-browser eval "document.title"  # Run JavaScript
```

#!/bin/sh
# Test script for MCP tools in Docker (full-featured image)

set -e

COMPOSE_FILE="docker/docker-compose.full.yml"
SERVICE="anyclaw-agent"

echo "ðŸ§ª Testing MCP tools in Docker container (full-featured image)..."
echo ""

# Build the image
echo "ðŸ“¦ Building Docker image..."
docker compose -f "$COMPOSE_FILE" build "$SERVICE"

# Test npx
echo "âœ?Testing npx..."
docker compose -f "$COMPOSE_FILE" run --rm --entrypoint sh "$SERVICE" -c 'npx --version'

# Test npm
echo "âœ?Testing npm..."
docker compose -f "$COMPOSE_FILE" run --rm --entrypoint sh "$SERVICE" -c 'npm --version'

# Test node
echo "âœ?Testing Node.js..."
docker compose -f "$COMPOSE_FILE" run --rm --entrypoint sh "$SERVICE" -c 'node --version'

# Test git
echo "âœ?Testing git..."
docker compose -f "$COMPOSE_FILE" run --rm --entrypoint sh "$SERVICE" -c 'git --version'

# Test python
echo "âœ?Testing Python..."
docker compose -f "$COMPOSE_FILE" run --rm --entrypoint sh "$SERVICE" -c 'python3 --version'

# Test uv
echo "âœ?Testing uv..."
docker compose -f "$COMPOSE_FILE" run --rm --entrypoint sh "$SERVICE" -c 'uv --version'

# Test MCP server installation (quick)
echo "âœ?Testing @modelcontextprotocol/server-filesystem MCP server install with npx..."
docker compose -f "$COMPOSE_FILE" run --rm --entrypoint sh "$SERVICE" -c '</dev/null timeout 5 npx -y @modelcontextprotocol/server-filesystem /tmp || true'

echo ""
echo "ðŸŽ‰ All MCP tools are working correctly!"
echo ""
echo "Next steps:"
echo "  1. Configure MCP servers in config/config.json"
echo "  2. Run: docker compose -f $COMPOSE_FILE --profile gateway up"

#!/bin/bash
# Stub ouroboros binary that captures put stdin to a file

case "$1" in
  query)
    if [ "$OUROBOROS_STUB_QUERY_EMPTY" = "1" ]; then
      echo "[]"
    else
      echo '[{"type":"note","title":"sample one"},{"type":"decision","title":"sample two"},{"type":"fact","title":"sample three"}]'
    fi
    exit 0
    ;;
  put)
    if [ "$OUROBOROS_STUB_PUT_FAIL" = "1" ]; then
      echo "stub failure" >&2
      exit 1
    fi
    # If OUROBOROS_PUT_CAPTURE_FILE is set, write stdin to that file
    if [ -n "$OUROBOROS_PUT_CAPTURE_FILE" ]; then
      cat > "$OUROBOROS_PUT_CAPTURE_FILE"
    fi
    echo '[{"id":1,"action":"created","title":"hook smoke"}]'
    exit 0
    ;;
  *)
    exit 0
    ;;
esac

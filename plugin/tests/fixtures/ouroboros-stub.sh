#!/bin/bash
# Stub ouroboros binary for testing

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
    echo '[{"id":1,"action":"created","title":"hook smoke"}]'
    exit 0
    ;;
  items)
    if [ "$OUROBOROS_STUB_ITEMS_EMPTY" = "1" ]; then
      echo "[]"
    else
      echo '[{"id":"OU-1","title":"test item","priority":"P2","status":"open","project":"test-project"}]'
    fi
    exit 0
    ;;
  *)
    exit 0
    ;;
esac

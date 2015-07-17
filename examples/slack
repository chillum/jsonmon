#! /bin/sh

# This script provides Slack notifications.
# Be sure to change the Slack URL to the actual Webhook URL.

# First arg is "true" or "false", this is the "failed" check value.
# The second one contains check's name.
# The third one is an error description.

case $1 in
    "true")
        data="{\"text\": \"Failed: $2\n$3\", \"icon_emoji\": \":bell\"}"
        ;;
    "false")
        data="{\"text\": \"Fixed: $2\", \"icon_emoji\": \":no_bell\"}"
        ;;
    *)
        exit 1
        ;;
esac

curl -H "Content-Type:application/json" -d "$data" https://hooks.slack.com/services/INTEGRATION_PATH

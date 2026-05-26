// Package slack indexes Slack workspace messages for RAG search.
// It talks to the Slack Web API exclusively through Nango's proxy
// boundary — the connector never sees or stores credentials.
//
// Only channels where the bot is already a member (is_member=true)
// are indexed. No automatic channel-join is attempted.
//
// Each Slack thread becomes one Document; each message in the thread
// is one Section. Text is cleaned via SlackTextCleaner (user IDs,
// channel refs, and special mentions are replaced with human-readable
// equivalents) before indexing.
package slack

# Guides

The behaviour behind the [tool reference](../tools.md): the shapes tools return, the identifiers they accept, and the Telegram rules that leak through into their parameters.

## Sections

<div class="grid cards" markdown>

-   :material-message-text:{ .lg .middle } **Messages**

    ---

    The output format of every read tool, the required `parseMode`, and the CommonMark subset's known limits.

    [:octicons-arrow-right-24: Messages](messages.md)

-   :material-account-box:{ .lg .middle } **Peers**

    ---

    The one identifier shape used everywhere, and how `@username`, numeric IDs and invite links resolve.

    [:octicons-arrow-right-24: Peers](peers.md)

-   :material-magnify:{ .lg .middle } **Search**

    ---

    Server-side kind filters, the global compound cursor, and history pagination.

    [:octicons-arrow-right-24: Search](search.md)

-   :material-file-tree:{ .lg .middle } **Resources and Prompts**

    ---

    Every resource and prompt the server registers, and subscribing to a chat's new messages.

    [:octicons-arrow-right-24: Resources and Prompts](resources.md)

-   :material-bullhorn:{ .lg .middle } **Posting as a channel**

    ---

    The `sendAs` identity, the chat default, and what MTProto will not let a client do.

    [:octicons-arrow-right-24: Posting as a channel](send-as.md)

-   :material-emoticon-happy:{ .lg .middle } **Reactions**

    ---

    Standard and custom-emoji encoding, and why a reactor is not always a user.

    [:octicons-arrow-right-24: Reactions](reactions.md)

</div>

# Quote lifecycle

Coti keeps the full commercial outcome on the same quote. A customer can accept, reject, or request
changes from the public quote link. A seller can also record an acceptance or rejection received by
phone, WhatsApp, or another channel from the order detail screen.

After reviewing a quote, the seller can send its link by email, WhatsApp, or both. The original
channel is selected by default when the order arrives through email or WhatsApp.

A change request, from the link or as a WhatsApp reply to a sent quote, moves the quote to "Change
requested" and opens one editable draft. Coti's AI reads the customer's messages and proposes the
changes on that draft; prices and discounts are always computed by Coti's own rules, and the seller
reviews the draft before anything is sent back. When a message is not a clear change — "ok, I'll
take it", a question about delivery — the AI changes nothing and tells the seller it needs them.
The order detail shows the whole conversation: what the customer wrote, how the AI read each message
and what the seller did with it. A seller who disagrees with the AI's changes can discard them and
start again from the version the customer received.

Accepted and rejected quotes remain auditable and cannot be edited directly. A seller can reactivate
one in either of two ways:

- Reuse the closed version and make it ready to resend without changing its products or prices.
- Create a new editable version, refreshed with current branch prices, then review and send it.

Every status change appears in the order tracking history. Reactivation always keeps the original
RFQ and quote relationship; it never creates a duplicate quote for the same request.

Archiving only removes a quote from the active queue. It does not change whether the quote was
accepted, rejected, or in progress, and restoring it keeps the same lifecycle state. An archived
quote can be read but not changed, sent, or invoiced until it is restored.

Coti also archives on its own a quote that has gone quiet: one still being prepared or ready to
send, or one sent whose validity has already run out, with no movement for a configurable number of
days (30 by default). A sent quote the customer can still accept is never archived automatically. If
a seller archives one by hand, the customer's link says the quote is no longer available. Quotes with a change request pending or already accepted or rejected are never archived
automatically. Archived quotes stay available under the "Archivado" status filter of the order list.

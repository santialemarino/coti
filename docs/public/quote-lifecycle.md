# Quote lifecycle

Coti keeps the full commercial outcome on the same quote. A customer can accept, reject, or request
changes from the public quote link. A seller can also record an acceptance or rejection received by
phone, WhatsApp, or another channel from the order detail screen.

Accepted and rejected quotes remain auditable and cannot be edited directly. A seller can reactivate
one in either of two ways:

- Reuse the closed version and make it ready to resend without changing its products or prices.
- Create a new editable version, refreshed with current branch prices, then review and send it.

Every status change appears in the order tracking history. Reactivation always keeps the original
RFQ and quote relationship; it never creates a duplicate quote for the same request.

Archiving only removes a quote from the active queue. It does not change whether the quote was
accepted, rejected, or in progress, and restoring it keeps the same lifecycle state. An archived
quote can be read but not changed, sent, or invoiced until it is restored.

Coti also archives on its own a quote that has gone quiet: one still being prepared, ready to send,
or sent and waiting for the customer, with no movement for a configurable number of days (30 by
default). Quotes with a change request pending or already accepted or rejected are never archived
automatically. Archived quotes stay available under the "Archivado" status filter of the order list.

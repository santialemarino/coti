'use client';

import { useCallback, useEffect, useId, useState } from 'react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { Badge, Button, DropdownChevron } from '@repo/ui/components';
import { SPRING } from '@repo/ui/lib';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { usePendingRouter } from '@/hooks/use-pending-router';
import { errorCodeOf } from '@/lib/api/errors';
import {
  createOrderFromNonOrder,
  dismissNonOrder,
  fetchNonOrders,
  type NonOrderMessage,
} from '@/lib/api/inbound-messages-client';
import { useFormatters } from '@/lib/i18n/formatters';

interface NonOrderMessagesProps {
  branchId: string;
  // Changes whenever the queue is read again, so the messages refresh with it.
  refreshKey: unknown;
  onOrderCreated: () => Promise<void>;
}

/*
 * «Mensajes sin pedido»: greetings, questions and automated mail that opened no order, folded under
 * the queue until shown. The seller opens an order from one the reading got wrong, or puts it away.
 */
export function NonOrderMessages({ branchId, refreshKey, onOrderCreated }: NonOrderMessagesProps) {
  const t = useTranslations('rfqs.nonOrders');
  const message = useApiErrorMessage('rfqs');
  const fmt = useFormatters();
  const router = usePendingRouter();
  const reduced = useReducedMotion();
  const headingId = useId();
  const listId = useId();

  const [messages, setMessages] = useState<NonOrderMessage[]>([]);
  const [expanded, setExpanded] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setMessages(await fetchNonOrders(branchId));
    } catch {
      // A failed background read keeps the last list rather than emptying it.
    }
  }, [branchId]);

  useEffect(() => {
    void load();
  }, [load, refreshKey]);

  async function dismiss(item: NonOrderMessage) {
    setBusy(item.id);
    try {
      await dismissNonOrder(item.id, branchId);
      setMessages((previous) => previous.filter((entry) => entry.id !== item.id));
      toast.success(t('dismissed'));
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    } finally {
      setBusy(null);
    }
  }

  async function createOrder(item: NonOrderMessage) {
    setBusy(item.id);
    try {
      const draft = await createOrderFromNonOrder(item.id, branchId);
      // Navigates before the list drops the message: the last one leaving unmounts this section.
      router.push(ROUTES.rfqsDetail(draft.rfq.id));
      await onOrderCreated();
      setMessages((previous) => previous.filter((entry) => entry.id !== item.id));
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    } finally {
      setBusy(null);
    }
  }

  if (messages.length === 0) return null;

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-y-1.5">
      <div className="flex min-h-7 items-center justify-between pl-3 gap-x-2">
        <h3
          id={headingId}
          className="flex items-center gap-x-1.5 text-paragraph-xs-medium text-foreground-muted"
        >
          <span
            aria-hidden="true"
            className="size-1.5 shrink-0 bg-foreground-subtle rounded-full"
          />
          {t('title')}
          <span className="text-foreground-subtle tabular-nums">{messages.length}</span>
        </h3>
        <button
          type="button"
          aria-expanded={expanded}
          aria-controls={listId}
          onClick={() => setExpanded((open) => !open)}
          className="group/toggle flex items-center px-1 gap-x-1 rounded-sm outline-none transition-colors duration-200 ease-out-soft hover:text-foreground focus-visible:text-foreground text-paragraph-mini-medium text-foreground-muted"
        >
          {t(expanded ? 'hide' : 'show')}
          <DropdownChevron
            open={expanded}
            className="size-3.5 group-focus-visible/toggle:animate-focus-bump-soft"
          />
        </button>
      </div>
      <AnimatePresence initial={false}>
        {expanded ? (
          <motion.ul
            key="messages"
            id={listId}
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            exit={{
              opacity: 0,
              height: 0,
              transition: reduced ? { duration: 0 } : SPRING.collapse,
            }}
            transition={reduced ? { duration: 0 } : SPRING.expand}
            className="flex flex-col gap-y-1.5 overflow-hidden"
          >
            {messages.map((item) => (
              <li
                key={item.id}
                className="flex flex-col px-3 py-2.5 gap-y-1.5 bg-background border border-border rounded-lg shadow-e1"
              >
                <span className="flex items-center justify-between gap-x-2">
                  <span
                    title={item.senderId}
                    className="truncate text-paragraph-sm-medium text-foreground"
                  >
                    {item.senderLabel ?? item.senderId}
                  </span>
                  <Badge tone="neutral" size="sm">
                    {t(`kind.${item.kind}`)}
                  </Badge>
                </span>
                <p className="line-clamp-3 whitespace-pre-line break-words text-paragraph-xs text-foreground-muted">
                  {item.body}
                </p>
                <span className="text-paragraph-mini text-foreground-subtle tabular-nums">
                  {fmt.timestamp(item.receivedAt)}
                </span>
                <span className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={busy !== null}
                    onClick={() => void createOrder(item)}
                  >
                    {t('createOrder')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={busy !== null}
                    onClick={() => void dismiss(item)}
                  >
                    {t('dismiss')}
                  </Button>
                </span>
              </li>
            ))}
          </motion.ul>
        ) : null}
      </AnimatePresence>
    </section>
  );
}

'use client';

import * as React from 'react';

import { Button } from './button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './dialog';
import { PendingButton } from './pending-button';

interface ConfirmDialogProps<TEntity> {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /* The subject of the confirmation. May go null while the dialog animates out. */
  entity: TEntity | null;
  title: string;
  description: (entity: TEntity) => string;
  onConfirm: () => void | Promise<void>;
  pending?: boolean;
  labels: { confirm: string; pending: string; cancel: string };
  tone?: 'default' | 'danger';
}

/*
 * The last non-null entity is held in a ref so the copy survives the close animation. A caller that
 * clears its selected row on close would otherwise blank the description mid-exit, and the dialog
 * visibly shrinks to an empty box before it fades — the single most common way a confirm dialog
 * looks broken.
 */
function ConfirmDialog<TEntity>({
  open,
  onOpenChange,
  entity,
  title,
  description,
  onConfirm,
  pending = false,
  labels,
  tone = 'danger',
}: ConfirmDialogProps<TEntity>) {
  const lastEntity = React.useRef(entity);
  if (entity) lastEntity.current = entity;
  const shown = entity ?? lastEntity.current;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md" closeOnClickOutside={!pending}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{shown ? description(shown) : ''}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" disabled={pending} onClick={() => onOpenChange(false)}>
            {labels.cancel}
          </Button>
          <PendingButton
            variant={tone === 'danger' ? 'destructive' : 'default'}
            pending={pending}
            pendingLabel={labels.pending}
            onClick={onConfirm}
          >
            {labels.confirm}
          </PendingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export { ConfirmDialog };

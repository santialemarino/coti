'use client';

import { useState, useTransition } from 'react';
import { PlusIcon, TagsIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Callout,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Checkbox,
  Input,
  Label,
  PendingButton,
} from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { ClientTag } from '@/lib/api/client-profiles';
import { createClientTag, replaceClientTags } from '@/lib/api/clients-client';
import { errorCodeOf } from '@/lib/api/errors';

interface ClientTagEditorProps {
  clientId: string;
  initialTags: ClientTag[];
  initialAvailable: ClientTag[];
}

export function ClientTagEditor({ clientId, initialTags, initialAvailable }: ClientTagEditorProps) {
  const t = useTranslations('clients.tags');
  const message = useApiErrorMessage('clients.tags.errors');
  const [available, setAvailable] = useState(initialAvailable);
  const [selected, setSelected] = useState(() => new Set(initialTags.map((tag) => tag.id)));
  const [newName, setNewName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [creating, startCreate] = useTransition();
  const [saving, startSave] = useTransition();

  function toggle(tagId: string, checked: boolean) {
    setSelected((current) => {
      const next = new Set(current);
      if (checked) next.add(tagId);
      else next.delete(tagId);
      return next;
    });
  }

  function createTag() {
    const name = newName.trim();
    if (!name) return;
    setError(null);
    startCreate(async () => {
      try {
        const tag = await createClientTag(name);
        setAvailable((current) =>
          current.some((item) => item.id === tag.id)
            ? current
            : [...current, tag].sort((a, b) => a.name.localeCompare(b.name, 'es-AR')),
        );
        setSelected((current) => new Set(current).add(tag.id));
        setNewName('');
      } catch (cause) {
        setError(message(errorCodeOf(cause)));
      }
    });
  }

  function save() {
    setError(null);
    startSave(async () => {
      try {
        await replaceClientTags(clientId, [...selected]);
        toast.success(t('saved'));
      } catch (cause) {
        setError(message(errorCodeOf(cause)));
      }
    });
  }

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between">
        <div className="flex items-center gap-x-2">
          <TagsIcon aria-hidden="true" className="size-4 text-foreground-muted" />
          <CardTitle>{t('title')}</CardTitle>
        </div>
        <Badge tone="neutral">{t('selected', { total: selected.size })}</Badge>
      </CardHeader>
      <CardContent className="flex flex-col gap-y-5">
        {error ? <Callout tone="danger">{error}</Callout> : null}
        <div className="flex flex-wrap gap-2">
          {available.map((tag) => (
            <Label
              key={tag.id}
              className="flex cursor-pointer items-center gap-x-2 rounded-lg border border-border px-3 py-2 transition-[border-color,background-color] duration-200 ease-out-soft hover:border-border-strong hover:bg-muted active:bg-surface-hover"
            >
              <Checkbox
                checked={selected.has(tag.id)}
                onCheckedChange={(checked) => toggle(tag.id, checked === true)}
                disabled={saving}
              />
              <span className="text-paragraph-sm text-foreground">{tag.name}</span>
            </Label>
          ))}
        </div>
        <div className="flex flex-col sm:flex-row gap-2">
          <Input
            value={newName}
            onChange={(event) => setNewName(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault();
                createTag();
              }
            }}
            maxLength={128}
            placeholder={t('newPlaceholder')}
            disabled={creating || saving}
          />
          <Button
            variant="outline"
            onClick={createTag}
            disabled={!newName.trim() || creating || saving}
          >
            <PlusIcon aria-hidden="true" />
            {t(creating ? 'creating' : 'create')}
          </Button>
        </div>
        <div className="flex justify-end">
          <PendingButton pending={saving} pendingLabel={t('saving')} onClick={save}>
            {t('save')}
          </PendingButton>
        </div>
      </CardContent>
    </Card>
  );
}

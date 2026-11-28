'use client';

import { useState, useTransition } from 'react';
import { useRouter } from 'next/navigation';
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
import {
  createClientTag,
  replaceClientTags,
  setClientTagExclusion,
} from '@/lib/api/clients-client';
import { errorCodeOf } from '@/lib/api/errors';

interface ClientTagEditorProps {
  clientId: string;
  initialTags: ClientTag[];
  initialAvailable: ClientTag[];
  excludedTags: ClientTag[];
}

export function ClientTagEditor({
  clientId,
  initialTags,
  initialAvailable,
  excludedTags,
}: ClientTagEditorProps) {
  const router = useRouter();
  const t = useTranslations('clients.tags');
  const message = useApiErrorMessage('clients.tags.errors');
  const [available, setAvailable] = useState(initialAvailable);
  const [selected, setSelected] = useState(
    () => new Set(initialTags.filter((tag) => !tag.automatic).map((tag) => tag.id)),
  );
  const [newName, setNewName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [creating, startCreate] = useTransition();
  const [saving, startSave] = useTransition();

  const [updatingExclusion, startExclusion] = useTransition();
  const activeAutomatic = initialTags.filter((tag) => tag.automatic);
  const busy = creating || saving || updatingExclusion;

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

  function changeExclusion(tagId: string, excluded: boolean) {
    setError(null);
    startExclusion(async () => {
      try {
        await setClientTagExclusion(clientId, tagId, excluded);
        router.refresh();
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
        <Badge tone="neutral">
          {t('selected', { total: selected.size + activeAutomatic.length })}
        </Badge>
      </CardHeader>
      <CardContent className="flex flex-col gap-y-5">
        {error ? <Callout tone="danger">{error}</Callout> : null}
        {activeAutomatic.length > 0 || excludedTags.length > 0 ? (
          <ul className="flex flex-wrap gap-2">
            {activeAutomatic.map((tag) => (
              <li key={tag.id} className="flex items-center gap-x-1">
                <Badge tone="neutral">{tag.name}</Badge>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  aria-label={t('excludeLabel', { name: tag.name })}
                  onClick={() => changeExclusion(tag.id, true)}
                >
                  {t('exclude')}
                </Button>
              </li>
            ))}
            {excludedTags.map((tag) => (
              <li key={tag.id} className="flex items-center gap-x-1">
                <Badge tone="outline">{t('excluded', { name: tag.name })}</Badge>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  aria-label={t('restoreLabel', { name: tag.name })}
                  onClick={() => changeExclusion(tag.id, false)}
                >
                  {t('restore')}
                </Button>
              </li>
            ))}
          </ul>
        ) : null}
        <div className="flex flex-wrap gap-2">
          {available.map((tag) => (
            <Label
              key={tag.id}
              className="flex cursor-pointer items-center gap-x-2 rounded-lg border border-border px-3 py-2 transition-[border-color,background-color] duration-200 ease-out-soft hover:border-border-strong hover:bg-muted active:bg-surface-hover"
            >
              <Checkbox
                checked={selected.has(tag.id)}
                onCheckedChange={(checked) => toggle(tag.id, checked === true)}
                disabled={busy}
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
            disabled={busy}
          />
          <Button variant="outline" onClick={createTag} disabled={!newName.trim() || busy}>
            <PlusIcon aria-hidden="true" />
            {t(creating ? 'creating' : 'create')}
          </Button>
        </div>
        <div className="flex justify-end">
          <PendingButton pending={saving} disabled={busy} pendingLabel={t('saving')} onClick={save}>
            {t('save')}
          </PendingButton>
        </div>
      </CardContent>
    </Card>
  );
}

'use client';

import { useEffect, useMemo, useState, useTransition } from 'react';
import Link from 'next/link';
import {
  BadgePlusIcon,
  Building2Icon,
  PencilIcon,
  PlusIcon,
  UserRoundCheckIcon,
} from 'lucide-react';
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  InlineLink,
  Input,
  Label,
  MetaList,
  PendingButton,
  RadioGroup,
  RadioGroupItem,
  SearchInput,
  Spinner,
  ToggleGroup,
  ToggleGroupItem,
} from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import {
  clientDisplayName,
  type ClientMatch,
  type ClientTag,
  type QuoteClientAssociation,
} from '@/lib/api/client-profiles';
import {
  associateQuoteClient,
  createClientTag,
  getClientDirectory,
  getQuoteClientAssociation,
} from '@/lib/api/clients-client';
import { errorCodeOf } from '@/lib/api/errors';

interface ClientAssociationCardProps {
  quoteId: string;
  branchId: string;
}

type AssociationMode = 'existing' | 'new';

export function ClientAssociationCard({ quoteId, branchId }: ClientAssociationCardProps) {
  const t = useTranslations('clients.association');
  const message = useApiErrorMessage('clients.association.errors');
  const [association, setAssociation] = useState<QuoteClientAssociation | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);

  useEffect(() => {
    let active = true;
    getQuoteClientAssociation(quoteId, branchId)
      .then((result) => {
        if (active) setAssociation(result);
      })
      .catch((cause) => {
        if (active) setError(message(errorCodeOf(cause)));
      });
    return () => {
      active = false;
    };
  }, [branchId, message, quoteId]);

  function onAssociated(match: ClientMatch, availableTags: ClientTag[]) {
    setAssociation((current) =>
      current
        ? { ...current, currentClient: match, availableTags }
        : {
            currentClient: match,
            suggestions: [],
            contactHints: { phone: match.client.phone, email: match.client.email },
            availableTags,
          },
    );
    toast.success(t('toast.saved', { name: clientDisplayName(match.client, t('unnamed')) }));
  }

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between gap-x-4">
        <div className="flex items-center gap-x-2">
          <UserRoundCheckIcon aria-hidden="true" className="size-4 text-foreground-muted" />
          <CardTitle>{t('title')}</CardTitle>
        </div>
        {association ? (
          <Button variant="outline" size="sm" onClick={() => setDialogOpen(true)}>
            {association.currentClient ? (
              <PencilIcon aria-hidden="true" />
            ) : (
              <PlusIcon aria-hidden="true" />
            )}
            {t(association.currentClient ? 'edit' : 'associate')}
          </Button>
        ) : null}
      </CardHeader>
      <CardContent>
        {error ? (
          <Callout tone="danger">{error}</Callout>
        ) : !association ? (
          <div className="flex items-center gap-x-2 text-paragraph-sm text-foreground-muted">
            <Spinner size="sm" />
            {t('loading')}
          </div>
        ) : association.currentClient ? (
          <div className="flex flex-col gap-y-3">
            <div className="flex flex-col gap-y-1">
              <InlineLink asChild>
                <Link href={`/clients/${association.currentClient.client.id}`}>
                  {clientDisplayName(association.currentClient.client, t('unnamed'))}
                </Link>
              </InlineLink>
              <MetaList
                items={[
                  association.currentClient.client.phone,
                  association.currentClient.client.email,
                  !association.currentClient.client.phone && !association.currentClient.client.email
                    ? t('noContact')
                    : null,
                ]}
              />
            </div>
            <TagList tags={association.currentClient.tags} emptyLabel={t('tags.none')} />
          </div>
        ) : (
          <Callout tone="warning" title={t('pending.title')}>
            {t('pending.description')}
          </Callout>
        )}
      </CardContent>

      {association ? (
        <ClientAssociationDialog
          open={dialogOpen}
          onOpenChange={setDialogOpen}
          quoteId={quoteId}
          branchId={branchId}
          association={association}
          onAssociated={onAssociated}
        />
      ) : null}
    </Card>
  );
}

interface ClientAssociationDialogProps extends ClientAssociationCardProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  association: QuoteClientAssociation;
  onAssociated: (match: ClientMatch, availableTags: ClientTag[]) => void;
}

function ClientAssociationDialog({
  open,
  onOpenChange,
  quoteId,
  branchId,
  association,
  onAssociated,
}: ClientAssociationDialogProps) {
  const t = useTranslations('clients.association');
  const message = useApiErrorMessage('clients.association.errors');
  const exactCandidates = useMemo(() => {
    const values = [association.currentClient, ...association.suggestions].filter(
      (candidate): candidate is ClientMatch => candidate !== null,
    );
    return values.filter(
      (candidate, index) =>
        values.findIndex((item) => item.client.id === candidate.client.id) === index,
    );
  }, [association.currentClient, association.suggestions]);
  const initialCandidate = association.currentClient ?? exactCandidates[0] ?? null;
  const [mode, setMode] = useState<AssociationMode>('existing');
  const [clientId, setClientId] = useState(initialCandidate?.client.id ?? '');
  const [selectedTags, setSelectedTags] = useState(
    () => new Set(initialCandidate?.tags.map((tag) => tag.id) ?? []),
  );
  const [availableTags, setAvailableTags] = useState(association.availableTags);
  const [name, setName] = useState('');
  const [phone, setPhone] = useState(association.contactHints.phone ?? '');
  const [email, setEmail] = useState(association.contactHints.email ?? '');
  const [directory, setDirectory] = useState<ClientMatch[]>([]);
  const [directoryQuery, setDirectoryQuery] = useState('');
  const [directoryLoading, setDirectoryLoading] = useState(false);
  const [directoryError, setDirectoryError] = useState<string | null>(null);
  const [newTag, setNewTag] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [creatingTag, startCreateTag] = useTransition();
  const [saving, startSave] = useTransition();

  const selectableCandidates = useMemo(() => {
    const values = [...exactCandidates, ...directory];
    return values.filter(
      (candidate, index) =>
        values.findIndex((item) => item.client.id === candidate.client.id) === index,
    );
  }, [directory, exactCandidates]);
  const directoryResults = useMemo(() => {
    const query = normalizeClientSearch(directoryQuery);
    if (!query) return [];
    const exactIDs = new Set(exactCandidates.map((candidate) => candidate.client.id));
    return directory.filter(
      (candidate) => !exactIDs.has(candidate.client.id) && clientMatchesSearch(candidate, query),
    );
  }, [directory, directoryQuery, exactCandidates]);

  useEffect(() => {
    if (!open) return;
    const candidate = association.currentClient ?? exactCandidates[0] ?? null;
    setMode('existing');
    setClientId(candidate?.client.id ?? '');
    setSelectedTags(new Set(candidate?.tags.map((tag) => tag.id) ?? []));
    setAvailableTags(association.availableTags);
    setName('');
    setPhone(association.contactHints.phone ?? '');
    setEmail(association.contactHints.email ?? '');
    setDirectoryQuery('');
    setDirectoryError(null);
    setNewTag('');
    setError(null);
  }, [association, exactCandidates, open]);

  useEffect(() => {
    if (!open) return;
    let active = true;
    setDirectory([]);
    setDirectoryLoading(true);
    setDirectoryError(null);
    getClientDirectory(branchId)
      .then((clients) => {
        if (active) setDirectory(clients);
      })
      .catch((cause) => {
        if (active) setDirectoryError(message(errorCodeOf(cause)));
      })
      .finally(() => {
        if (active) setDirectoryLoading(false);
      });
    return () => {
      active = false;
    };
  }, [branchId, message, open]);

  function chooseCandidate(id: string) {
    setClientId(id);
    const candidate = selectableCandidates.find((item) => item.client.id === id);
    setSelectedTags(new Set(candidate?.tags.map((tag) => tag.id) ?? []));
  }

  function changeDirectoryQuery(value: string) {
    setDirectoryQuery(value);
    const selectedIsExact = exactCandidates.some((candidate) => candidate.client.id === clientId);
    if (clientId && !selectedIsExact) {
      setClientId('');
      setSelectedTags(new Set());
    }
  }

  function chooseMode(value: string) {
    if (!value) return;
    const nextMode = value as AssociationMode;
    setMode(nextMode);
    if (nextMode === 'new') {
      setSelectedTags(new Set());
      return;
    }
    const candidate =
      selectableCandidates.find((item) => item.client.id === clientId) ?? exactCandidates[0];
    if (candidate) {
      setClientId(candidate.client.id);
      setSelectedTags(new Set(candidate.tags.map((tag) => tag.id)));
    }
  }

  function toggleTag(tagId: string, checked: boolean) {
    setSelectedTags((current) => {
      const next = new Set(current);
      if (checked) next.add(tagId);
      else next.delete(tagId);
      return next;
    });
  }

  function createTag() {
    const value = newTag.trim();
    if (!value) return;
    setError(null);
    startCreateTag(async () => {
      try {
        const tag = await createClientTag(value);
        setAvailableTags((current) =>
          current.some((item) => item.id === tag.id)
            ? current
            : [...current, tag].sort((a, b) => a.name.localeCompare(b.name, 'es-AR')),
        );
        setSelectedTags((current) => new Set(current).add(tag.id));
        setNewTag('');
      } catch (cause) {
        setError(message(errorCodeOf(cause)));
      }
    });
  }

  function submit() {
    setError(null);
    startSave(async () => {
      try {
        const body =
          mode === 'existing'
            ? { client_id: clientId, tag_ids: [...selectedTags] }
            : {
                new_client: {
                  name: name.trim() || undefined,
                  phone: phone.trim() || undefined,
                  email: email.trim() || undefined,
                },
                tag_ids: [...selectedTags],
              };
        const match = await associateQuoteClient(quoteId, branchId, body);
        onAssociated(match, availableTags);
        onOpenChange(false);
      } catch (cause) {
        setError(message(errorCodeOf(cause)));
      }
    });
  }

  const canSubmit =
    mode === 'existing' ? Boolean(clientId) : Boolean(name.trim() || phone.trim() || email.trim());

  return (
    <Dialog open={open} onOpenChange={(next) => !saving && onOpenChange(next)}>
      <DialogContent className="sm:max-w-2xl" closeOnClickOutside={false}>
        <DialogHeader>
          <DialogTitle>{t('dialog.title')}</DialogTitle>
          <DialogDescription>{t('dialog.description')}</DialogDescription>
        </DialogHeader>

        {error ? <Callout tone="danger">{error}</Callout> : null}

        <ToggleGroup type="single" value={mode} onValueChange={chooseMode} className="w-full">
          <ToggleGroupItem value="existing">
            <UserRoundCheckIcon aria-hidden="true" />
            {t('dialog.existing')}
          </ToggleGroupItem>
          <ToggleGroupItem value="new">
            <Building2Icon aria-hidden="true" />
            {t('dialog.new')}
          </ToggleGroupItem>
        </ToggleGroup>

        {mode === 'existing' ? (
          <div className="flex flex-col gap-y-5">
            {exactCandidates.length > 0 ? (
              <div className="flex flex-col gap-y-3">
                <Label>{t('dialog.chooseClient')}</Label>
                <RadioGroup value={clientId} onValueChange={chooseCandidate}>
                  {exactCandidates.map((candidate) => (
                    <ClientOption
                      key={candidate.client.id}
                      candidate={candidate}
                      currentClientId={association.currentClient?.client.id}
                      unnamedLabel={t('unnamed')}
                      noContactLabel={t('noContact')}
                      currentLabel={t('dialog.current')}
                    />
                  ))}
                </RadioGroup>
              </div>
            ) : null}

            <div
              className={
                exactCandidates.length > 0
                  ? 'flex flex-col gap-y-3 border-t border-border pt-4'
                  : 'flex flex-col gap-y-3'
              }
            >
              <Label htmlFor="association-client-search">{t('dialog.searchClient')}</Label>
              <SearchInput
                id="association-client-search"
                value={directoryQuery}
                onChange={(event) => changeDirectoryQuery(event.target.value)}
                onClear={() => changeDirectoryQuery('')}
                clearLabel={t('dialog.clearSearch')}
                placeholder={t('dialog.searchPlaceholder')}
                containerClassName="w-full"
                disabled={saving}
              />
              {directoryLoading ? (
                <div className="flex items-center gap-x-2 text-paragraph-sm text-foreground-muted">
                  <Spinner size="sm" />
                  {t('dialog.searchLoading')}
                </div>
              ) : directoryError ? (
                <Callout tone="danger">{directoryError}</Callout>
              ) : directoryQuery.trim() && directoryResults.length === 0 ? (
                <p className="text-paragraph-sm text-foreground-muted">{t('dialog.searchEmpty')}</p>
              ) : directoryResults.length > 0 ? (
                <RadioGroup
                  value={clientId}
                  onValueChange={chooseCandidate}
                  className="max-h-56 overflow-y-auto pr-1"
                >
                  {directoryResults.map((candidate) => (
                    <ClientOption
                      key={candidate.client.id}
                      candidate={candidate}
                      currentClientId={association.currentClient?.client.id}
                      unnamedLabel={t('unnamed')}
                      noContactLabel={t('noContact')}
                      currentLabel={t('dialog.current')}
                    />
                  ))}
                </RadioGroup>
              ) : null}
            </div>
          </div>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="flex flex-col gap-y-2 sm:col-span-2">
              <Label htmlFor="association-client-name">{t('dialog.name')}</Label>
              <Input
                id="association-client-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                maxLength={255}
                placeholder={t('dialog.namePlaceholder')}
                disabled={saving}
              />
            </div>
            <div className="flex flex-col gap-y-2">
              <Label htmlFor="association-client-phone">{t('dialog.phone')}</Label>
              <Input
                id="association-client-phone"
                value={phone}
                onChange={(event) => setPhone(event.target.value)}
                maxLength={64}
                placeholder="+5491122334455"
                disabled={saving}
              />
            </div>
            <div className="flex flex-col gap-y-2">
              <Label htmlFor="association-client-email">{t('dialog.email')}</Label>
              <Input
                id="association-client-email"
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                maxLength={255}
                placeholder="cliente@empresa.com"
                disabled={saving}
              />
            </div>
            <p className="sm:col-span-2 text-paragraph-xs text-foreground-muted">
              {t('dialog.newHint')}
            </p>
          </div>
        )}

        <div className="flex flex-col gap-y-3 border-t border-border pt-4">
          <div className="flex items-center gap-x-2">
            <BadgePlusIcon aria-hidden="true" className="size-4 text-foreground-muted" />
            <Label>{t('tags.title')}</Label>
          </div>
          <div className="flex flex-wrap gap-2">
            {availableTags.map((tag) => (
              <Label
                key={tag.id}
                className="flex cursor-pointer items-center gap-x-2 rounded-lg border border-border px-3 py-2 transition-[border-color,background-color] duration-200 ease-out-soft hover:border-border-strong hover:bg-muted active:bg-surface-hover"
              >
                <Checkbox
                  checked={selectedTags.has(tag.id)}
                  onCheckedChange={(checked) => toggleTag(tag.id, checked === true)}
                  disabled={saving}
                />
                <span className="text-paragraph-sm text-foreground">{tag.name}</span>
              </Label>
            ))}
          </div>
          <div className="flex flex-col sm:flex-row gap-2">
            <Input
              value={newTag}
              onChange={(event) => setNewTag(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault();
                  createTag();
                }
              }}
              maxLength={128}
              placeholder={t('tags.newPlaceholder')}
              disabled={creatingTag || saving}
            />
            <Button
              variant="outline"
              onClick={createTag}
              disabled={!newTag.trim() || creatingTag || saving}
            >
              <PlusIcon aria-hidden="true" />
              {t(creatingTag ? 'tags.creating' : 'tags.create')}
            </Button>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            {t('dialog.cancel')}
          </Button>
          <PendingButton
            pending={saving}
            pendingLabel={t('dialog.saving')}
            onClick={submit}
            disabled={!canSubmit}
          >
            {t('dialog.save')}
          </PendingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function TagList({ tags, emptyLabel }: { tags: ClientTag[]; emptyLabel: string }) {
  if (tags.length === 0) {
    return <span className="text-paragraph-sm text-foreground-subtle">{emptyLabel}</span>;
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {tags.map((tag) => (
        <Badge key={tag.id} tone="outline" size="sm">
          {tag.name}
        </Badge>
      ))}
    </div>
  );
}

interface ClientOptionProps {
  candidate: ClientMatch;
  currentClientId?: string;
  unnamedLabel: string;
  noContactLabel: string;
  currentLabel: string;
}

function ClientOption({
  candidate,
  currentClientId,
  unnamedLabel,
  noContactLabel,
  currentLabel,
}: ClientOptionProps) {
  const { client } = candidate;
  return (
    <Label className="flex w-full cursor-pointer items-start gap-x-3 rounded-lg border border-border px-3 py-3 transition-[border-color,background-color] duration-200 ease-out-soft hover:border-border-strong hover:bg-muted active:bg-surface-hover">
      <RadioGroupItem value={client.id} className="mt-0.5" />
      <span className="flex min-w-0 flex-1 flex-col gap-y-1">
        <span className="text-paragraph-sm-medium text-foreground">
          {clientDisplayName(client, unnamedLabel)}
        </span>
        <MetaList
          className="text-paragraph-xs"
          items={[
            client.phone,
            client.email,
            !client.phone && !client.email ? noContactLabel : null,
          ]}
        />
      </span>
      {currentClientId === client.id ? (
        <Badge tone="success" size="sm">
          {currentLabel}
        </Badge>
      ) : null}
    </Label>
  );
}

function clientMatchesSearch(candidate: ClientMatch, normalizedQuery: string): boolean {
  const { client } = candidate;
  return [client.name, client.phone, client.email].some(
    (value) => value && normalizeClientSearch(value).includes(normalizedQuery),
  );
}

function normalizeClientSearch(value: string): string {
  return value
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLocaleLowerCase('es-AR')
    .replace(/[^a-z0-9]/g, '');
}

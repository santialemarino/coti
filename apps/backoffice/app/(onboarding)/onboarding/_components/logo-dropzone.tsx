'use client';

import { useEffect, useRef, useState } from 'react';
import Image from 'next/image';
import { ImageIcon, UploadCloudIcon, XIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, Callout } from '@repo/ui/components';

const ACCEPTED_TYPES = ['image/png', 'image/jpeg', 'image/svg+xml'];

interface LogoDropzoneProps {
  onPreviewChange?: (url: string | null) => void;
}

export function LogoDropzone({ onPreviewChange }: LogoDropzoneProps) {
  const t = useTranslations('onboarding.brand.logo');
  const inputRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!file) {
      setPreviewUrl(null);
      onPreviewChange?.(null);
      return;
    }
    const url = URL.createObjectURL(file);
    setPreviewUrl(url);
    onPreviewChange?.(url);
    return () => URL.revokeObjectURL(url);
  }, [file, onPreviewChange]);

  function choose(next: File | undefined) {
    if (!next) return;
    if (!ACCEPTED_TYPES.includes(next.type)) {
      setError(t('invalidType'));
      return;
    }
    setError(null);
    setFile(next);
  }

  function onDrop(event: React.DragEvent<HTMLDivElement>) {
    event.preventDefault();
    choose(event.dataTransfer.files[0]);
  }

  function clear() {
    setFile(null);
    setError(null);
    if (inputRef.current) inputRef.current.value = '';
  }

  return (
    <div className="flex flex-col gap-y-3">
      <div
        onDragOver={(event) => event.preventDefault()}
        onDrop={onDrop}
        className="flex flex-col min-h-52 items-center justify-center px-6 py-7 gap-y-4 bg-card border-2 border-dashed border-strong rounded-1.5xl"
      >
        <input
          ref={inputRef}
          type="file"
          accept="image/png,image/jpeg,image/svg+xml"
          className="sr-only"
          onChange={(event) => choose(event.target.files?.[0])}
        />
        {previewUrl ? (
          <div className="flex h-24 w-64 items-center justify-center p-3 bg-background border rounded-lg relative">
            <Image
              src={previewUrl}
              alt={t('previewAlt')}
              fill
              unoptimized
              className="p-3 object-contain"
            />
          </div>
        ) : (
          <span className="flex size-12 items-center justify-center bg-accent rounded-full text-accent-foreground">
            <UploadCloudIcon aria-hidden="true" className="size-6" />
          </span>
        )}
        <div className="flex flex-col items-center gap-y-1 text-center">
          <p className="text-paragraph-medium">{file?.name ?? t('title')}</p>
          <p className="text-paragraph-sm text-foreground-muted">
            {file ? t('localOnly') : t('hint')}
          </p>
        </div>
        <div className="flex items-center gap-x-2">
          <Button type="button" variant="outline" onClick={() => inputRef.current?.click()}>
            <ImageIcon aria-hidden="true" />
            {file ? t('replace') : t('choose')}
          </Button>
          {file ? (
            <Button type="button" variant="ghost" onClick={clear}>
              <XIcon aria-hidden="true" />
              {t('remove')}
            </Button>
          ) : null}
        </div>
      </div>
      {error ? <Callout tone="danger">{error}</Callout> : null}
    </div>
  );
}

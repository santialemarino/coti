'use client';

import { useEffect, useState } from 'react';
import Image from 'next/image';
import { ImageIcon, UploadCloudIcon, XIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, Callout } from '@repo/ui/components';
import { FileDropzone } from '@/components/file-dropzone';

const ACCEPTED_TYPES = ['image/png', 'image/jpeg', 'image/svg+xml'];

interface LogoDropzoneProps {
  onPreviewChange?: (url: string | null) => void;
}

export function LogoDropzone({ onPreviewChange }: LogoDropzoneProps) {
  const t = useTranslations('onboarding.brand.logo');
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

  function clear() {
    setFile(null);
    setError(null);
  }

  return (
    <div className="flex flex-col gap-y-3">
      <FileDropzone
        accept="image/png,image/jpeg,image/svg+xml"
        onFile={choose}
        className="min-h-52 py-7"
      >
        {({ dragging, openFileDialog }) => (
          <>
            {previewUrl ? (
              <div className="relative flex h-24 w-full max-w-64 items-center justify-center p-3 bg-background border rounded-lg">
                <Image
                  src={previewUrl}
                  alt={t('previewAlt')}
                  fill
                  unoptimized
                  className="p-3 object-contain"
                />
              </div>
            ) : (
              <span
                className="flex size-12 items-center justify-center bg-accent rounded-full text-accent-foreground transition-[scale,translate] duration-200 ease-out-soft data-[dragging=true]:scale-110 data-[dragging=true]:-translate-y-1"
                data-dragging={dragging}
              >
                <UploadCloudIcon aria-hidden="true" className="size-6" />
              </span>
            )}
            <div className="flex flex-col items-center gap-y-1 text-center">
              <p className="break-all text-paragraph-medium">
                {dragging ? t('release') : (file?.name ?? t('title'))}
              </p>
              <p className="text-paragraph-sm text-foreground-muted">
                {file ? t('localOnly') : t('hint')}
              </p>
            </div>
            <div className="flex flex-col w-full items-stretch gap-2 sm:flex-row sm:w-auto sm:items-center">
              <Button type="button" variant="outline" onClick={openFileDialog}>
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
          </>
        )}
      </FileDropzone>
      {error ? <Callout tone="danger">{error}</Callout> : null}
    </div>
  );
}

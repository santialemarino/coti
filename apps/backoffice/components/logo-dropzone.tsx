'use client';

import { useEffect, useState } from 'react';
import Image from 'next/image';
import { ImageIcon, UploadCloudIcon, XIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, Callout } from '@repo/ui/components';
import { FileDropzone } from '@/components/file-dropzone';

const ACCEPTED_TYPES = ['image/png', 'image/jpeg'];

interface LogoDropzoneProps {
  initialUrl?: string | null;
  onFileChange?: (file: File | null) => void;
  onPreviewChange?: (url: string | null) => void;
}

export function LogoDropzone({ initialUrl, onFileChange, onPreviewChange }: LogoDropzoneProps) {
  const t = useTranslations('common.logoUpload');
  const [selection, setSelection] = useState<File | null>();
  const [previewUrl, setPreviewUrl] = useState<string | null>(initialUrl ?? null);
  const [error, setError] = useState<string | null>(null);
  const file = selection instanceof File ? selection : null;

  useEffect(() => {
    if (selection === undefined) {
      setPreviewUrl(initialUrl ?? null);
      return;
    }
    if (selection === null) {
      setPreviewUrl(null);
      return;
    }
    const url = URL.createObjectURL(selection);
    setPreviewUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [initialUrl, selection]);

  useEffect(() => onPreviewChange?.(previewUrl), [onPreviewChange, previewUrl]);

  function choose(next: File | undefined) {
    if (!next) return;
    if (!ACCEPTED_TYPES.includes(next.type)) {
      setError(t('invalidType'));
      return;
    }
    setError(null);
    setSelection(next);
    onFileChange?.(next);
  }

  function clear() {
    setSelection(null);
    setError(null);
    onFileChange?.(null);
  }

  return (
    <div className="flex flex-col gap-y-3">
      <FileDropzone accept="image/png,image/jpeg" onFile={choose} className="min-h-52 py-7">
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
            </div>
            <div className="flex flex-col w-full items-stretch gap-2 sm:flex-row sm:w-auto sm:items-center">
              <Button type="button" variant="outline" onClick={openFileDialog}>
                <ImageIcon aria-hidden="true" />
                {previewUrl ? t('replace') : t('choose')}
              </Button>
              {previewUrl ? (
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

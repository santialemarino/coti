'use client';

import { useEffect, useRef, useState } from 'react';
import { useTranslations } from 'next-intl';

import {
  Button,
  Callout,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  PendingButton,
  ToggleGroup,
  ToggleGroupItem,
} from '@repo/ui/components';
import { clampLogoOffset, logoCropGeometry, type LogoOffset } from '@/lib/utils/logo-crop';

const SHAPES = ['original', 'square', 'wide'] as const;
type Shape = (typeof SHAPES)[number];

// The wide frame's ratio, also the stand-in for "original" until the image has loaded.
const WIDE_RATIO = 3;
// The preview is this tall at most and never wider than the dialog's content.
const PREVIEW_HEIGHT = 280;
const PREVIEW_MAX_WIDTH = 600;
// The exported PNG's longest side.
const EXPORT_MAX_SIDE = 1200;
// One arrow press pans this fraction of the frame.
const KEY_PAN_STEP = 0.03;
const ZOOM_MIN = 1;
const ZOOM_MAX = 4;
const ZOOM_STEP = 0.05;
const CENTERED: LogoOffset = { x: 0, y: 0 };

const KEY_DIRECTIONS: Record<string, [number, number]> = {
  ArrowLeft: [-KEY_PAN_STEP, 0],
  ArrowRight: [KEY_PAN_STEP, 0],
  ArrowUp: [0, -KEY_PAN_STEP],
  ArrowDown: [0, KEY_PAN_STEP],
};

interface LogoCropDialogProps {
  open: boolean;
  file: File;
  onCancel: () => void;
  onConfirm: (file: File) => void;
}

export function LogoCropDialog({ open, file, onCancel, onConfirm }: LogoCropDialogProps) {
  const t = useTranslations('common.logoUpload.crop');
  const canvas = useRef<HTMLCanvasElement>(null);
  const drag = useRef<{ x: number; y: number; offset: LogoOffset } | null>(null);
  const [image, setImage] = useState<HTMLImageElement | null>(null);
  const [shape, setShape] = useState<Shape>('original');
  const [zoom, setZoom] = useState(ZOOM_MIN);
  const [offset, setOffset] = useState<LogoOffset>(CENTERED);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState(false);

  const ratio =
    shape === 'square'
      ? 1
      : shape === 'original' && image
        ? image.naturalWidth / image.naturalHeight
        : WIDE_RATIO;
  const width = Math.max(1, Math.round(Math.min(PREVIEW_MAX_WIDTH, PREVIEW_HEIGHT * ratio)));
  const height = Math.max(1, Math.round(width / ratio));

  useEffect(() => {
    const url = URL.createObjectURL(file);
    const source = new window.Image();
    let active = true;
    source.onload = () => {
      if (active) setImage(source);
    };
    source.onerror = () => {
      if (active) setError(true);
    };
    source.src = url;
    return () => {
      active = false;
      URL.revokeObjectURL(url);
    };
  }, [file]);

  useEffect(() => {
    const context = canvas.current?.getContext('2d');
    if (!context || !image) return;
    const geometry = logoCropGeometry(
      image.naturalWidth,
      image.naturalHeight,
      width,
      height,
      zoom,
      offset.x,
      offset.y,
    );
    context.clearRect(0, 0, width, height);
    context.drawImage(image, geometry.x, geometry.y, geometry.width, geometry.height);
  }, [image, width, height, zoom, offset]);

  function clamped(next: LogoOffset, nextZoom = zoom): LogoOffset {
    if (!image) return CENTERED;
    return clampLogoOffset(image.naturalWidth, image.naturalHeight, width, height, nextZoom, next);
  }

  function reset(nextShape = shape) {
    setError(false);
    setShape(nextShape);
    setZoom(ZOOM_MIN);
    setOffset(CENTERED);
  }

  function changeZoom(nextZoom: number) {
    setZoom(nextZoom);
    setOffset((current) => clamped(current, nextZoom));
  }

  async function confirm() {
    if (!image) return;
    setPending(true);
    setError(false);
    try {
      const output = document.createElement('canvas');
      const outputWidth = Math.max(
        1,
        Math.round(Math.min(EXPORT_MAX_SIDE, EXPORT_MAX_SIDE * ratio)),
      );
      const outputHeight = Math.max(1, Math.round(outputWidth / ratio));
      output.width = outputWidth;
      output.height = outputHeight;
      const context = output.getContext('2d');
      if (!context) throw new Error('Canvas unavailable');
      const geometry = logoCropGeometry(
        image.naturalWidth,
        image.naturalHeight,
        outputWidth,
        outputHeight,
        zoom,
        offset.x,
        offset.y,
      );
      context.drawImage(image, geometry.x, geometry.y, geometry.width, geometry.height);
      const blob = await new Promise<Blob>((resolve, reject) =>
        output.toBlob(
          (value) => (value ? resolve(value) : reject(new Error('Logo export failed'))),
          'image/png',
        ),
      );
      onConfirm(
        new File([blob], `${file.name.replace(/\.[^.]+$/, '')}.png`, { type: 'image/png' }),
      );
    } catch {
      setError(true);
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && !pending && onCancel()}>
      <DialogContent className="sm:max-w-lg" closeOnClickOutside={!pending}>
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <ToggleGroup
          type="single"
          size="sm"
          value={shape}
          onValueChange={(value) => value && reset(value as Shape)}
          disabled={pending}
          aria-label={t('shape')}
        >
          {SHAPES.map((value) => (
            <ToggleGroupItem key={value} value={value}>
              {t(value)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <div className="flex min-h-48 items-center justify-center p-3 bg-sunken border border-border rounded-lg">
          <canvas
            ref={canvas}
            width={width}
            height={height}
            role="img"
            aria-label={t('preview')}
            tabIndex={0}
            className="max-w-full bg-background rounded-sm outline-none cursor-grab touch-none transition-[box-shadow] duration-200 ease-out-soft focus-visible:ring-3 focus-visible:ring-ring/45 active:cursor-grabbing"
            onPointerDown={(event) => {
              if (!image || pending) return;
              event.currentTarget.setPointerCapture(event.pointerId);
              drag.current = { x: event.clientX, y: event.clientY, offset };
            }}
            onPointerMove={(event) => {
              const start = drag.current;
              if (!start) return;
              const rect = event.currentTarget.getBoundingClientRect();
              setOffset(
                clamped({
                  x: start.offset.x + (event.clientX - start.x) / rect.width,
                  y: start.offset.y + (event.clientY - start.y) / rect.height,
                }),
              );
            }}
            onPointerUp={() => {
              drag.current = null;
            }}
            onPointerCancel={() => {
              drag.current = null;
            }}
            onKeyDown={(event) => {
              const direction = KEY_DIRECTIONS[event.key];
              if (!direction || pending) return;
              event.preventDefault();
              setOffset((current) =>
                clamped({ x: current.x + direction[0], y: current.y + direction[1] }),
              );
            }}
          />
        </div>
        <div className="flex flex-col gap-y-2">
          <div className="relative flex h-8 items-center">
            <div
              aria-hidden="true"
              className="absolute inset-x-2 h-1.5 bg-border rounded-full overflow-hidden pointer-events-none"
            >
              <div
                className="h-full bg-primary"
                style={{ width: `${((zoom - ZOOM_MIN) / (ZOOM_MAX - ZOOM_MIN)) * 100}%` }}
              />
            </div>
            <input
              id="logo-crop-zoom"
              aria-label={t('zoom')}
              type="range"
              min={ZOOM_MIN}
              max={ZOOM_MAX}
              step={ZOOM_STEP}
              value={zoom}
              disabled={!image || pending}
              onChange={(event) => changeZoom(Number(event.target.value))}
              className="relative m-0 h-8 w-full cursor-pointer appearance-none rounded-full bg-transparent outline-none focus-visible:ring-3 focus-visible:ring-ring/45 disabled:cursor-not-allowed disabled:opacity-50 [&::-webkit-slider-runnable-track]:h-1.5 [&::-webkit-slider-runnable-track]:bg-transparent [&::-webkit-slider-thumb]:-mt-1.25 [&::-webkit-slider-thumb]:size-4 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-primary hover:[&::-webkit-slider-thumb]:bg-primary-hover active:[&::-webkit-slider-thumb]:bg-primary-active [&::-moz-range-track]:h-1.5 [&::-moz-range-track]:bg-transparent [&::-moz-range-thumb]:size-4 [&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:border-0 [&::-moz-range-thumb]:bg-primary hover:[&::-moz-range-thumb]:bg-primary-hover active:[&::-moz-range-thumb]:bg-primary-active"
            />
          </div>
          <p className="text-paragraph-xs text-foreground-muted">{t('moveHint')}</p>
        </div>
        {error ? <Callout tone="danger">{t('error')}</Callout> : null}
        <DialogFooter>
          <Button type="button" variant="ghost" disabled={pending} onClick={() => reset()}>
            {t('reset')}
          </Button>
          <Button type="button" variant="outline" disabled={pending} onClick={onCancel}>
            {t('cancel')}
          </Button>
          <PendingButton
            type="button"
            pending={pending}
            pendingLabel={t('saving')}
            disabled={!image}
            onClick={confirm}
          >
            {t('confirm')}
          </PendingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

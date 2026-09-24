'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import Image from 'next/image';
import { zodResolver } from '@hookform/resolvers/zod';
import { ImageIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Button,
  Combobox,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Dropzone,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Input,
  PendingButton,
  Textarea,
} from '@repo/ui/components';
import { productSchema, type ProductValues } from '@/app/(protected)/settings/catalog/form-schema';
import type { Product, ProductFamily } from '@/lib/api/products';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface ProductFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: 'create' | 'edit';
  product: Product | null;
  families: ProductFamily[];
  pending: boolean;
  onSubmit: (values: ProductValues, image: File | null) => void;
}

export function ProductFormDialog({
  open,
  onOpenChange,
  mode,
  product,
  families,
  pending,
  onSubmit,
}: ProductFormDialogProps) {
  const t = useTranslations('products');
  const tErrors = useTranslations('common.form.errors');
  const schema = useMemo(() => productSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const lastMode = useRef(mode);
  if (open) lastMode.current = mode;
  const copy = open ? mode : lastMode.current;
  const [image, setImage] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const form = useForm<ProductValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: emptyProduct(),
  });
  const familyId = form.watch('familyId');
  const subgroups = families.find((family) => family.id === familyId)?.subgroups ?? [];

  useEffect(() => {
    if (!open) return;
    form.reset({
      code: product?.code ?? '',
      name: product?.name ?? '',
      description: product?.description ?? '',
      unit: product?.unit ?? '',
      familyId: product?.familyId ?? '',
      subgroupId: product?.subgroupId ?? '',
      isActive: product?.isActive ?? true,
    });
    setImage(null);
    setPreviewUrl(product?.imageUrl ?? null);
  }, [open, product, form]);

  useEffect(() => {
    if (!image) return;
    const url = URL.createObjectURL(image);
    setPreviewUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [image]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent closeOnClickOutside={!pending}>
        <DialogHeader>
          <DialogTitle>{t(`${copy}.title`)}</DialogTitle>
          <DialogDescription>{t(`${copy}.description`)}</DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit((values) => onSubmit(values, image))}
            noValidate
            className="flex flex-col gap-y-5"
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('name.label')}</FormLabel>
                    <FormControl>
                      <Input
                        maxLength={TEXT_FIELD_MAX_LENGTH}
                        placeholder={t('name.placeholder')}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="code"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('code.label')}</FormLabel>
                    <FormControl>
                      <Input
                        maxLength={TEXT_FIELD_MAX_LENGTH}
                        placeholder={t('code.placeholder')}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name="description"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('description.label')}</FormLabel>
                  <FormControl>
                    <Textarea
                      maxLength={512}
                      placeholder={t('description.placeholder')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className="grid gap-4 sm:grid-cols-2">
              <FormField
                control={form.control}
                name="unit"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('unit.label')}</FormLabel>
                    <FormControl>
                      <Input maxLength={64} placeholder={t('unit.placeholder')} {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="familyId"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('family.label')}</FormLabel>
                    <FormControl>
                      <Combobox
                        value={field.value || null}
                        onValueChange={(value) => {
                          field.onChange(value);
                          form.setValue('subgroupId', '');
                        }}
                        options={families.map((family) => ({
                          value: family.id,
                          label: family.name,
                        }))}
                        placeholder={t('family.placeholder')}
                        searchable={families.length > 8}
                        searchPlaceholder={t('family.search')}
                        emptyLabel={t('family.empty')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name="subgroupId"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('subgroup.label')}</FormLabel>
                  <FormControl>
                    <Combobox
                      value={field.value || null}
                      onValueChange={field.onChange}
                      options={subgroups.map((subgroup) => ({
                        value: subgroup.id,
                        label: subgroup.name,
                      }))}
                      placeholder={t('subgroup.placeholder')}
                      disabled={!familyId || subgroups.length === 0}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className="flex flex-col gap-y-2">
              <span className="text-paragraph-sm-medium">{t('image.label')}</span>
              <Dropzone
                accept="image/png,image/jpeg,image/webp"
                disabled={pending}
                onFile={(file) => setImage(file ?? null)}
                title={t('image.title')}
                releaseLabel={t('image.release')}
                chooseLabel={previewUrl ? t('image.replace') : t('image.choose')}
                hint={t('image.hint')}
                fileName={image?.name}
                preview={
                  previewUrl ? (
                    <Image
                      src={previewUrl}
                      alt=""
                      width={96}
                      height={96}
                      unoptimized
                      className="size-24 object-cover rounded-xl"
                    />
                  ) : undefined
                }
                icon={ImageIcon}
              />
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={pending}
                onClick={() => onOpenChange(false)}
              >
                {t('cancel')}
              </Button>
              <PendingButton type="submit" pending={pending} pendingLabel={t(`${copy}.submitting`)}>
                {t(`${copy}.submit`)}
              </PendingButton>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

function emptyProduct(): ProductValues {
  return {
    code: '',
    name: '',
    description: '',
    unit: '',
    familyId: '',
    subgroupId: '',
    isActive: true,
  };
}

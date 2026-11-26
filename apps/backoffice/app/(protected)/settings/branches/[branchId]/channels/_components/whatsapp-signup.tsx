'use client';

import { useEffect, useRef, useState, useTransition } from 'react';
import { useRouter } from 'next/navigation';
import Script from 'next/script';
import { useTranslations } from 'next-intl';

import { Button, Callout } from '@repo/ui/components';
import { connectWhatsAppSignup } from '@/app/(protected)/settings/branches/[branchId]/channels/actions';
import type { BranchChannel, WhatsAppSignupSettings } from '@/lib/api/branch-channels';

type MetaSDK = {
  init(options: { appId: string; version: string; autoLogAppEvents: boolean }): void;
  login(
    callback: (response?: { authResponse?: { code?: string } }) => void,
    options: {
      config_id: string;
      response_type: 'code';
      override_default_response_type: true;
      extras: { setup: Record<string, never> };
    },
  ): void;
};

type Flow = {
  code?: string;
  businessAccountId?: string;
  phoneNumberId?: string;
  timer?: ReturnType<typeof setTimeout>;
  removeListener?: () => void;
};

let activeFlow: Flow | null = null;

function readMetaEvent(event: MessageEvent) {
  const allowed = /^https:\/\/(?:[a-z0-9-]+\.)*facebook\.com$/i;
  if (!allowed.test(event.origin) || typeof event.data !== 'string') return null;
  try {
    const message = JSON.parse(event.data);
    if (message?.type !== 'WA_EMBEDDED_SIGNUP') return null;
    if (message.event === 'CANCEL' || message.event === 'ERROR') {
      return { type: message.event as 'CANCEL' | 'ERROR' };
    }
    const phoneNumberId = message.data?.phone_number_id;
    const businessAccountId = message.data?.waba_id;
    if (
      ![
        'FINISH',
        'FINISH_WHATSAPP_BUSINESS_APP_ONBOARDING',
        'FINISH_GRANT_ONLY_API_ACCESS',
      ].includes(message.event) ||
      typeof phoneNumberId !== 'string' ||
      !/^\d{1,32}$/.test(phoneNumberId) ||
      typeof businessAccountId !== 'string' ||
      !/^\d{1,32}$/.test(businessAccountId)
    )
      return { type: 'ERROR' as const };
    return { type: 'FINISH' as const, phoneNumberId, businessAccountId };
  } catch {
    return null;
  }
}

interface WhatsAppSignupProps {
  channel: BranchChannel;
  settings: WhatsAppSignupSettings;
}

export function WhatsAppSignup({ channel, settings }: WhatsAppSignupProps) {
  const t = useTranslations('branchChannels.signup');
  const router = useRouter();
  const [checking, startCheck] = useTransition();
  const flowRef = useRef<Flow | null>(null);
  const sdkRef = useRef<MetaSDK | null>(null);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<'failed' | 'cancelled' | 'loadFailed' | 'conflict' | null>(
    null,
  );
  const { appId, configurationId, graphVersion, received } = settings;
  const enabled = !!(appId && configurationId && graphVersion);

  useEffect(
    () => () => {
      const flow = flowRef.current;
      if (activeFlow === flow) activeFlow = null;
      if (flow?.timer) clearTimeout(flow.timer);
      flow?.removeListener?.();
      flowRef.current = null;
    },
    [],
  );

  function release(flow: Flow) {
    if (flowRef.current !== flow) return false;
    if (flow.timer) clearTimeout(flow.timer);
    flow.removeListener?.();
    flowRef.current = null;
    activeFlow = null;
    setBusy(false);
    return true;
  }

  function complete(flow: Flow) {
    if (flowRef.current !== flow || !flow.code || !flow.phoneNumberId || !flow.businessAccountId)
      return;
    const result = {
      code: flow.code,
      businessAccountId: flow.businessAccountId,
      phoneNumberId: flow.phoneNumberId,
    };
    flow.code = undefined;
    flow.removeListener?.();
    if (flow.timer) clearTimeout(flow.timer);
    void connectWhatsAppSignup(channel.branchId, channel.id, result)
      .then((response) => {
        if (!release(flow)) return;
        if (response.ok) router.refresh();
        else setError(response.error === 'CONFLICT' ? 'conflict' : 'failed');
      })
      .catch(() => {
        if (release(flow)) setError('failed');
      });
  }

  function initialize() {
    const fb = (window as Window & { FB?: MetaSDK }).FB;
    if (!appId || !graphVersion || !fb) {
      setError('loadFailed');
      return;
    }
    try {
      fb.init({ appId, version: graphVersion, autoLogAppEvents: false });
      sdkRef.current = fb;
      setReady(true);
    } catch {
      setError('loadFailed');
    }
  }

  function connect() {
    const sdk = sdkRef.current;
    if (!sdk || !configurationId || activeFlow) return;
    const flow: Flow = {};
    flowRef.current = activeFlow = flow;
    setError(null);
    setBusy(true);
    function onMessage(event: MessageEvent) {
      const result = readMetaEvent(event);
      if (!result) return;
      if (result.type !== 'FINISH') {
        if (release(flow)) setError(result.type === 'CANCEL' ? 'cancelled' : 'failed');
        return;
      }
      flow.businessAccountId = result.businessAccountId;
      flow.phoneNumberId = result.phoneNumberId;
      complete(flow);
    }
    window.addEventListener('message', onMessage);
    flow.removeListener = () => window.removeEventListener('message', onMessage);
    try {
      sdk.login(
        (response) => {
          if (flowRef.current !== flow) return;
          if (!response?.authResponse?.code) {
            if (release(flow)) setError('cancelled');
            return;
          }
          flow.code = response.authResponse.code;
          flow.timer = setTimeout(() => {
            if (release(flow)) setError('failed');
          }, 8000);
          complete(flow);
        },
        {
          config_id: configurationId,
          response_type: 'code',
          override_default_response_type: true,
          extras: { setup: {} },
        },
      );
    } catch {
      if (release(flow)) setError('loadFailed');
    }
  }

  return (
    <div className="flex flex-col items-start gap-y-3">
      {enabled ? (
        <>
          <Script
            id="whatsapp-meta-sdk"
            src="https://connect.facebook.net/es_LA/sdk.js"
            strategy="afterInteractive"
            crossOrigin="anonymous"
            onReady={initialize}
            onError={() => setError('loadFailed')}
          />
          <Button type="button" onClick={connect} disabled={!ready || busy}>
            {t(busy ? 'connecting' : channel.isConfigured ? 'reconnect' : 'connect')}
          </Button>
          <p className="text-paragraph-sm text-foreground-muted">{t('metaHint')}</p>
        </>
      ) : null}
      {error ? <Callout tone="warning">{t(error)}</Callout> : null}
      {channel.isActive && channel.isConfigured ? (
        <div className="flex flex-col items-start gap-y-2">
          <p className="text-paragraph-sm text-foreground-muted" role="status">
            {t(received ? 'received' : 'awaiting')}
          </p>
          {!received ? (
            <Button
              type="button"
              variant="outline"
              disabled={checking}
              onClick={() => startCheck(() => router.refresh())}
            >
              {t(checking ? 'checking' : 'check')}
            </Button>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

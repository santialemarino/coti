export type ClientTagRuleKind = 'RECURRING' | 'ABANDONER';

export interface ClientTagRule {
  kind: ClientTagRuleKind;
  enabled: boolean;
  minimum: number;
  windowDays: number;
}

export interface ClientTagRuleRaw {
  kind: ClientTagRuleKind;
  enabled: boolean;
  minimum: number;
  window_days: number;
}

export function mapClientTagRule(rule: ClientTagRuleRaw): ClientTagRule {
  return {
    kind: rule.kind,
    enabled: rule.enabled,
    minimum: rule.minimum,
    windowDays: rule.window_days,
  };
}

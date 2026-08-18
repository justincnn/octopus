import { useEffect, useMemo, useState } from 'react';
import { Loader2, Zap, CheckCircle2, XCircle, RefreshCw, X, CircleDashed } from 'lucide-react';
import { useTranslations } from 'use-intl';
import { cn } from '@/lib/utils';
import type { KeyProbeStatus, ModelTestResult } from '@/api/endpoints/channel';
import { useTestChannelModels } from '@/api/endpoints/channel';
import { Button } from '@/components/ui/button';

interface ModelTestDialogProps {
    models: string[];
    /** 完整渠道配置(含明文 key) + 待测模型列表, 后端走真实 outbound 转换路径 */
    testPayload: TestPayload;
    onClose: () => void;
    /** 测试通过后一键把可用模型加入已选模型 */
    onAddAvailable: (models: string[]) => void;
}

type TestPayload = Parameters<ReturnType<typeof useTestChannelModels>['mutate']>[0];

type RowState =
    | { model: string; status: 'waiting' | 'testing' }
    | { model: string; status: 'done'; result: ModelTestResult };

/** 模型可用性测试弹窗: 打开即自动批量测试, 逐行显示状态/延迟/错误; 底部展示 key 池探测状态。 */
export function ModelTestDialog({ models, testPayload, onClose, onAddAvailable }: ModelTestDialogProps) {
    const t = useTranslations('channel.form');
    const testModels = useTestChannelModels();
    const [rows, setRows] = useState<RowState[]>([]);
    const [keyStatus, setKeyStatus] = useState<KeyProbeStatus[]>([]);

    const startTest = () => {
        if (models.length === 0) return;
        setRows(models.map((m) => ({ model: m, status: 'waiting' as const })));
        setKeyStatus([]);
        testModels.mutate(testPayload, {
            onSuccess: (data) => {
                const byName = new Map(data.results.map((r) => [r.model_name, r]));
                setRows((prev) =>
                    prev.map((row) =>
                        row.status === 'done' ? row
                            : byName.has(row.model) ? { model: row.model, status: 'done', result: byName.get(row.model)! }
                            : row
                    )
                );
                setKeyStatus(data.key_status ?? []);
            },
            onError: (error) => {
                setRows((prev) =>
                    prev.map((row) =>
                        row.status === 'done' ? row
                            : { model: row.model, status: 'done', result: { model_name: row.model, ok: false, latency_ms: 0, error: error instanceof Error ? error.message : String(error) } }
                    )
                );
            },
        });
    };

    useEffect(() => {
        if (models.length > 0) startTest();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const stats = useMemo(() => {
        const done = rows.filter((r) => r.status === 'done') as Extract<RowState, { status: 'done' }>[];
        const ok = done.filter((r) => r.result.ok).length;
        return { total: rows.length, done: done.length, ok };
    }, [rows]);

    // 测试通过的模型(去重保序)
    const okModels = useMemo(() => {
        const seen = new Set<string>();
        const out: string[] = [];
        rows.forEach((r) => {
            if (r.status === 'done' && r.result.ok && !seen.has(r.result.model_name)) {
                seen.add(r.result.model_name);
                out.push(r.result.model_name);
            }
        });
        return out;
    }, [rows]);

    return (
        <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 p-4">
            <div className="flex w-full max-w-md flex-col rounded-2xl border border-border bg-card p-4 text-card-foreground shadow-2xl">
                <div className="mb-2 flex items-center justify-between">
                    <h3 className="flex items-center gap-2 text-base font-bold">
                        <Zap className="size-4 text-primary" />
                        {t('modelTestTitle')}
                    </h3>
                    <button type="button" onClick={onClose} className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground">
                        <X className="size-4" />
                    </button>
                </div>

                <div className="mb-2 flex items-center justify-between text-xs text-muted-foreground">
                    <span>{t('modelTestProgress', { done: stats.done, total: stats.total })}</span>
                    {stats.done === stats.total && stats.total > 0 && (
                        <span className={cn('font-medium', stats.ok === stats.total ? 'text-primary' : 'text-destructive')}>
                            {t('modelTestSummary', { ok: stats.ok, total: stats.total })}
                        </span>
                    )}
                </div>

                <div className="max-h-72 space-y-1.5 overflow-y-auto pr-1">
                    {rows.length === 0 && (
                        <div className="py-6 text-center text-xs text-muted-foreground">{t('modelTestEmpty')}</div>
                    )}
                    {rows.map((row) => (
                        <div key={row.model} className="flex items-center gap-2 rounded-lg border border-border/50 bg-muted/30 px-2.5 py-2">
                            {row.status === 'done' && row.result.ok && <CheckCircle2 className="size-3.5 shrink-0 text-primary" />}
                            {row.status === 'done' && !row.result.ok && <XCircle className="size-3.5 shrink-0 text-destructive" />}
                            {row.status !== 'done' && <Loader2 className="size-3.5 shrink-0 animate-spin text-muted-foreground" />}
                            <div className="min-w-0 flex-1">
                                <div className="flex items-center gap-2">
                                    <span className="truncate text-xs font-medium">{row.model}</span>
                                    {row.status === 'done' && (
                                        <span className={cn('shrink-0 text-[10px]', row.result.ok ? 'text-primary' : 'text-destructive')}>
                                            {row.result.ok ? `${row.result.latency_ms}ms` : '失败'}
                                        </span>
                                    )}
                                </div>
                                {row.status === 'done' && (
                                    <div className={cn('truncate text-[10px]', row.result.ok ? 'text-muted-foreground' : 'text-destructive/80')}>
                                        {row.result.ok
                                            ? (row.result.key_used ? `key ${row.result.key_used} · ${row.result.content ?? ''}` : (row.result.content ?? ''))
                                            : (row.result.error ?? '')}
                                    </div>
                                )}
                            </div>
                        </div>
                    ))}
                </div>

                {/* key 池探测状态: 只标注被测过的 key, 未测 = unknown */}
                {keyStatus.length > 0 && (
                    <div className="mt-2 rounded-lg border border-border/50 bg-muted/20 p-2">
                        <div className="mb-1.5 text-[11px] font-medium text-muted-foreground">Key 池状态</div>
                        <div className="space-y-1">
                            {keyStatus.map((ks) => (
                                <div key={ks.key} className="flex items-center gap-2 text-[11px]">
                                    {ks.status === 'ok' && <CheckCircle2 className="size-3 shrink-0 text-primary" />}
                                    {ks.status === 'failed' && <XCircle className="size-3 shrink-0 text-destructive" />}
                                    {ks.status === 'unknown' && <CircleDashed className="size-3 shrink-0 text-muted-foreground/60" />}
                                    <span className={cn(
                                        'truncate font-mono',
                                        ks.status === 'failed' && 'text-destructive',
                                        ks.status === 'unknown' && 'text-muted-foreground/60'
                                    )}>
                                        {ks.key}
                                    </span>
                                    <span className={cn(
                                        'shrink-0',
                                        ks.status === 'ok' ? 'text-primary' : ks.status === 'failed' ? 'text-destructive' : 'text-muted-foreground/60'
                                    )}>
                                        {ks.status === 'ok' ? '可用' : ks.status === 'failed' ? (ks.error ? ks.error.slice(0, 60) : '失效') : '未测试'}
                                    </span>
                                </div>
                            ))}
                        </div>
                    </div>
                )}

                <div className="mt-3 flex justify-end gap-2">
                    {stats.ok > 0 && (
                        <Button
                            type="button"
                            size="sm"
                            className="h-8 px-3 text-xs"
                            onClick={() => onAddAvailable(okModels)}
                        >
                            <CheckCircle2 className="size-3 mr-1" />
                            {t('modelAddAvailable', { ok: okModels.length })}
                        </Button>
                    )}
                    <Button
                        type="button"
                        variant="secondary"
                        size="sm"
                        className="h-8 px-3 text-xs"
                        onClick={startTest}
                        disabled={testModels.isPending || models.length === 0}
                    >
                        <RefreshCw className={cn('size-3 mr-1', testModels.isPending && 'animate-spin')} />
                        {t('modelTestRetry')}
                    </Button>
                </div>
            </div>
        </div>
    );
}

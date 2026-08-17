import { useEffect, useState } from 'react';
import { useTranslations } from 'use-intl';
import { Search, Tags, Loader2, ChevronRight, Sparkles, Trash2, RefreshCw } from 'lucide-react';
import {
    useUnmatchedModels,
    useMatchModel,
    useMatchAll,
    useSetModelAlias,
    useDeleteModelAlias,
    useModelAliases,
    useSearchModels,
    type ModelMatchCandidate,
} from '@/api/endpoints/model';
import { toast } from '@/components/common/Toast';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

/** 单条候选行: 规范名 + 理由 + 价格 + 确认按钮(未匹配/已匹配/搜索结果共用) */
function CandidateRow({
    cand,
    disabled,
    onPick,
}: {
    cand: ModelMatchCandidate;
    disabled?: boolean;
    onPick: () => void;
}) {
    const t = useTranslations('model');
    return (
        <button
            type="button"
            disabled={disabled}
            onClick={onPick}
            className="w-full flex items-center gap-2 rounded-lg border border-border bg-card px-2.5 py-1.5 text-left transition-colors hover:border-primary/40 hover:bg-primary/5 disabled:opacity-50"
        >
            <span className="flex-1 min-w-0">
                <span className="block text-xs font-medium text-card-foreground font-mono truncate">
                    {cand.canonical_id}
                </span>
                <span className="block text-[10px] text-muted-foreground">{cand.reason}</span>
            </span>
            <span className="shrink-0 text-[10px] text-muted-foreground tabular-nums">
                ↓{cand.price.input.toFixed(2)} ↑{cand.price.output.toFixed(2)}
            </span>
            <span
                className={cn(
                    'shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-semibold',
                    'bg-primary/10 text-primary'
                )}
            >
                {t('unmatched.confirmBtn')}
            </span>
        </button>
    );
}

/** 关键词搜索全量价格目录: 输入 >=2 字符自动搜, 结果可点选 */
function SearchPanel({
    disabled,
    onPick,
}: {
    disabled?: boolean;
    onPick: (cand: ModelMatchCandidate) => void;
}) {
    const t = useTranslations('model');
    const [q, setQ] = useState('');
    const { data: results, isFetching } = useSearchModels(q);
    const show = q.trim().length >= 2;
    return (
        <div className="space-y-1">
            <div className="flex items-center gap-1.5">
                <Search className="size-3.5 text-muted-foreground shrink-0" />
                <input
                    value={q}
                    onChange={(e) => setQ(e.target.value)}
                    placeholder={t('unmatched.searchPlaceholder')}
                    className="w-full rounded-lg border border-border bg-card px-2 py-1 text-xs outline-none focus:border-primary/40"
                />
                {isFetching && <Loader2 className="size-3 animate-spin text-muted-foreground shrink-0" />}
            </div>
            {show && !isFetching && (!results || results.length === 0) && (
                <p className="text-xs text-muted-foreground pl-1">{t('unmatched.searchEmpty')}</p>
            )}
            {(results || []).map((cand) => (
                <CandidateRow key={cand.canonical_id} cand={cand} disabled={disabled} onPick={() => onPick(cand)} />
            ))}
        </div>
    );
}

/**
 * 未匹配/已匹配模型价格管理器: 未匹配 tab 自动匹配/批量匹配;
 * 已匹配 tab 全量别名列表, 支持重新匹配(自动候选+关键词搜索)与删除(回到未匹配池)。
 */
export function UnmatchedModelsDialog() {
    const t = useTranslations('model');
    const { data: unmatched, refetch } = useUnmatchedModels();
    const { data: aliases, refetch: refetchAliases } = useModelAliases();
    const match = useMatchModel();
    const matchAll = useMatchAll();
    const setAlias = useSetModelAlias();
    const delAlias = useDeleteModelAlias();
    const [tab, setTab] = useState<'unmatched' | 'matched'>('unmatched');
    const [expanded, setExpanded] = useState<Set<string>>(new Set());
    const [candidates, setCandidates] = useState<Record<string, ModelMatchCandidate[]>>({});
    const [loadingName, setLoadingName] = useState<string | null>(null);

    // 打开时自动刷新
    useEffect(() => {
        refetch();
        refetchAliases();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const handleMatch = (name: string) => {
        if (expanded.has(name)) {
            setExpanded((prev) => {
                const next = new Set(prev);
                next.delete(name);
                return next;
            });
            return;
        }
        setExpanded((prev) => new Set(prev).add(name));
        setLoadingName(name);
        match.mutate(name, {
            onSuccess: (res) => {
                setCandidates((prev) => ({ ...prev, [name]: res }));
            },
            onError: (err) => {
                setExpanded((prev) => {
                    const next = new Set(prev);
                    next.delete(name);
                    return next;
                });
                toast.error(t('toast.matchFailed'), { description: err.message });
            },
            onSettled: () => setLoadingName(null),
        });
    };

    const handleMatchAll = () => {
        matchAll.mutate(undefined, {
            onSuccess: (res) => {
                const map: Record<string, ModelMatchCandidate[]> = {};
                res.forEach((r) => {
                    map[r.name] = r.candidates;
                });
                setCandidates((prev) => ({ ...prev, ...map }));
                setExpanded((prev) => new Set([...prev, ...res.map((r) => r.name)]));
                if (res.length > 0) {
                    toast.success(`${res.length} 个模型已批量匹配，请逐条确认`);
                }
            },
            onError: (err) => toast.error(t('toast.matchFailed'), { description: err.message }),
        });
    };

    const handleConfirm = (src: string, candidate: ModelMatchCandidate) => {
        setAlias.mutate({ src, canonical: candidate.canonical_id }, {
            onSuccess: () => {
                toast.success(`${src} → ${candidate.canonical_id}`);
                setExpanded((prev) => {
                    const next = new Set(prev);
                    next.delete(src);
                    return next;
                });
            },
            onError: (err) => toast.error(t('toast.aliasSetFailed'), { description: err.message }),
        });
    };

    const handleDelete = (src: string) => {
        delAlias.mutate(src, {
            onSuccess: () => {
                toast.success(`${src} 已删除，回到未匹配池`);
                setExpanded((prev) => {
                    const next = new Set(prev);
                    next.delete(src);
                    return next;
                });
            },
            onError: (err) => toast.error(t('toast.aliasSetFailed'), { description: err.message }),
        });
    };

    const matchedEntries = Object.entries(aliases || {}).sort((a, b) => a[0].localeCompare(b[0]));
    const unmatchedCount = unmatched?.length || 0;
    const isEmpty = tab === 'unmatched' ? unmatchedCount === 0 : matchedEntries.length === 0;

    return (
        <div className="w-[min(92vw,560px)] max-h-[70vh] flex flex-col overflow-hidden">
            <div className="flex items-center justify-between pb-3">
                <h3 className="text-base font-bold text-card-foreground flex items-center gap-2">
                    <Tags className="size-4 text-primary" />
                    {t('unmatched.title')}
                </h3>
                <span className="text-xs text-muted-foreground">{t('unmatched.subtitle')}</span>
            </div>

            {/* tab 切换 */}
            <div className="flex items-center gap-1 pb-2">
                <button
                    type="button"
                    onClick={() => setTab('unmatched')}
                    className={cn(
                        'rounded-lg px-2.5 py-1 text-xs font-medium transition-colors',
                        tab === 'unmatched'
                            ? 'bg-primary/10 text-primary'
                            : 'text-muted-foreground hover:bg-muted/60'
                    )}
                >
                    {t('unmatched.tabUnmatched')}
                    {unmatchedCount > 0 && (
                        <span className="ml-1 rounded-full bg-destructive/10 px-1.5 py-0.5 text-[10px] font-semibold text-destructive">
                            {unmatchedCount}
                        </span>
                    )}
                </button>
                <button
                    type="button"
                    onClick={() => setTab('matched')}
                    className={cn(
                        'rounded-lg px-2.5 py-1 text-xs font-medium transition-colors',
                        tab === 'matched'
                            ? 'bg-primary/10 text-primary'
                            : 'text-muted-foreground hover:bg-muted/60'
                    )}
                >
                    {t('unmatched.tabMatched')}
                    {matchedEntries.length > 0 && (
                        <span className="ml-1 rounded-full bg-primary/10 px-1.5 py-0.5 text-[10px] font-semibold text-primary">
                            {matchedEntries.length}
                        </span>
                    )}
                </button>
                {tab === 'unmatched' && !isEmpty && (
                    <Button
                        variant="outline"
                        size="sm"
                        onClick={handleMatchAll}
                        disabled={matchAll.isPending}
                        className="rounded-lg ml-auto"
                    >
                        {matchAll.isPending ? (
                            <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                            <Sparkles className="size-3.5" />
                        )}
                        {t('unmatched.matchAllBtn')}
                    </Button>
                )}
            </div>

            <div className="flex-1 min-h-0 overflow-y-auto pr-1 space-y-1.5">
                {isEmpty ? (
                    <div className="flex flex-col items-center justify-center py-10 text-muted-foreground">
                        <Search className="size-8 mb-2 opacity-40" />
                        <span className="text-sm">{t('unmatched.empty')}</span>
                    </div>
                ) : tab === 'unmatched' ? (
                    (unmatched || []).map((name) => {
                        const isOpen = expanded.has(name);
                        const cands = candidates[name];
                        return (
                            <div key={name} className="rounded-xl border border-border bg-muted/10 overflow-hidden">
                                <div className="flex items-center gap-2 px-3 py-2">
                                    <span className="flex-1 min-w-0 truncate text-sm font-medium text-card-foreground font-mono">
                                        {name}
                                    </span>
                                    <Button
                                        variant="outline"
                                        size="sm"
                                        onClick={() => handleMatch(name)}
                                        disabled={loadingName === name}
                                        className="rounded-lg shrink-0"
                                    >
                                        {loadingName === name ? (
                                            <Loader2 className="size-3.5 animate-spin" />
                                        ) : isOpen ? (
                                            <ChevronRight className="size-3.5 rotate-90" />
                                        ) : (
                                            <Search className="size-3.5" />
                                        )}
                                        {t('unmatched.matchBtn')}
                                    </Button>
                                </div>
                                {isOpen && (
                                    <div className="px-3 pb-2 space-y-1">
                                        {!cands || cands.length === 0 ? (
                                            <p className="text-xs text-muted-foreground pl-1">{t('unmatched.noCandidate')}</p>
                                        ) : (
                                            cands.map((cand) => (
                                                <CandidateRow
                                                    key={cand.canonical_id}
                                                    cand={cand}
                                                    disabled={setAlias.isPending}
                                                    onPick={() => handleConfirm(name, cand)}
                                                />
                                            ))
                                        )}
                                        <SearchPanel disabled={setAlias.isPending} onPick={(cand) => handleConfirm(name, cand)} />
                                    </div>
                                )}
                            </div>
                        );
                    })
                ) : (
                    matchedEntries.map(([src, canonical]) => {
                        const isOpen = expanded.has(src);
                        const cands = candidates[src];
                        return (
                            <div key={src} className="rounded-xl border border-border bg-muted/10 overflow-hidden">
                                <div className="flex items-center gap-2 px-3 py-2">
                                    <span className="flex-1 min-w-0">
                                        <span className="block truncate text-sm font-medium text-card-foreground font-mono">
                                            {src}
                                        </span>
                                        <span className="block truncate text-[10px] text-muted-foreground font-mono">
                                            → {canonical}
                                        </span>
                                    </span>
                                    <Button
                                        variant="outline"
                                        size="sm"
                                        onClick={() => handleMatch(src)}
                                        disabled={loadingName === src}
                                        className="rounded-lg shrink-0"
                                    >
                                        {loadingName === src ? (
                                            <Loader2 className="size-3.5 animate-spin" />
                                        ) : isOpen ? (
                                            <ChevronRight className="size-3.5 rotate-90" />
                                        ) : (
                                            <RefreshCw className="size-3.5" />
                                        )}
                                        {t('unmatched.rematchBtn')}
                                    </Button>
                                    <Button
                                        variant="ghost"
                                        size="sm"
                                        onClick={() => handleDelete(src)}
                                        disabled={delAlias.isPending}
                                        className="rounded-lg shrink-0 text-destructive hover:text-destructive"
                                    >
                                        <Trash2 className="size-3.5" />
                                        {t('unmatched.deleteBtn')}
                                    </Button>
                                </div>
                                {isOpen && (
                                    <div className="px-3 pb-2 space-y-1">
                                        {!cands || cands.length === 0 ? (
                                            <p className="text-xs text-muted-foreground pl-1">{t('unmatched.noCandidate')}</p>
                                        ) : (
                                            cands.map((cand) => (
                                                <CandidateRow
                                                    key={cand.canonical_id}
                                                    cand={cand}
                                                    disabled={setAlias.isPending}
                                                    onPick={() => handleConfirm(src, cand)}
                                                />
                                            ))
                                        )}
                                        <SearchPanel disabled={setAlias.isPending} onPick={(cand) => handleConfirm(src, cand)} />
                                    </div>
                                )}
                            </div>
                        );
                    })
                )}
            </div>
        </div>
    );
}

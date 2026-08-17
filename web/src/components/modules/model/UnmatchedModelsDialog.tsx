import { useEffect, useState } from 'react';
import { useTranslations } from 'use-intl';
import { Search, Tags, Loader2, ChevronRight, X } from 'lucide-react';
import {
    useUnmatchedModels,
    useMatchModel,
    useSetModelAlias,
    type ModelMatchCandidate,
} from '@/api/endpoints/model';
import { toast } from '@/components/common/Toast';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

/**
 * 未匹配模型匹配器：列出所有渠道里没有价格的模型，逐个模糊匹配到 models.dev 规范名。
 * 确认后写入别名，价格自动跟上游同步。
 */
export function UnmatchedModelsDialog() {
    const t = useTranslations('model');
    const { data: unmatched, refetch } = useUnmatchedModels();
    const match = useMatchModel();
    const setAlias = useSetModelAlias();
    const [expanded, setExpanded] = useState<string | null>(null);
    const [candidates, setCandidates] = useState<Record<string, ModelMatchCandidate[]>>({});
    const [loadingName, setLoadingName] = useState<string | null>(null);

    // 打开时自动刷新数量
    useEffect(() => {
        refetch();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const handleMatch = (name: string) => {
        if (expanded === name) {
            setExpanded(null);
            return;
        }
        setLoadingName(name);
        match.mutate(name, {
            onSuccess: (res) => {
                setCandidates((prev) => ({ ...prev, [name]: res }));
                setExpanded(name);
            },
            onError: (err) => toast.error(t('toast.matchFailed'), { description: err.message }),
            onSettled: () => setLoadingName(null),
        });
    };

    const handleConfirm = (src: string, candidate: ModelMatchCandidate) => {
        setAlias.mutate({ src, canonical: candidate.canonical_id }, {
            onSuccess: () => {
                toast.success(`${src} → ${candidate.canonical_id}`);
                setExpanded(null);
            },
            onError: (err) => toast.error(t('toast.aliasSetFailed'), { description: err.message }),
        });
    };

    const items = unmatched || [];
    const isEmpty = items.length === 0;

    return (
        <div className="w-[min(92vw,560px)] max-h-[70vh] flex flex-col overflow-hidden">
            <div className="flex items-center justify-between pb-3">
                <h3 className="text-base font-bold text-card-foreground flex items-center gap-2">
                    <Tags className="size-4 text-primary" />
                    {t('unmatched.title')}
                    {!isEmpty && (
                        <span className="rounded-full bg-destructive/10 px-2 py-0.5 text-xs font-semibold text-destructive">
                            {items.length}
                        </span>
                    )}
                </h3>
                <span className="text-xs text-muted-foreground">{t('unmatched.subtitle')}</span>
            </div>

            <div className="flex-1 min-h-0 overflow-y-auto pr-1 space-y-1.5">
                {isEmpty ? (
                    <div className="flex flex-col items-center justify-center py-10 text-muted-foreground">
                        <Search className="size-8 mb-2 opacity-40" />
                        <span className="text-sm">{t('unmatched.empty')}</span>
                    </div>
                ) : (
                    items.map((name) => {
                        const isOpen = expanded === name;
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
                                                <button
                                                    key={cand.canonical_id}
                                                    type="button"
                                                    disabled={setAlias.isPending}
                                                    onClick={() => handleConfirm(name, cand)}
                                                    className="w-full flex items-center gap-2 rounded-lg border border-border bg-card px-2.5 py-1.5 text-left transition-colors hover:border-primary/40 hover:bg-primary/5 disabled:opacity-50"
                                                >
                                                    <span className="flex-1 min-w-0">
                                                        <span className="block text-xs font-medium text-card-foreground font-mono truncate">
                                                            {cand.canonical_id}
                                                        </span>
                                                        <span className="block text-[10px] text-muted-foreground">
                                                            {cand.reason}
                                                        </span>
                                                    </span>
                                                    <span className="shrink-0 text-[10px] text-muted-foreground tabular-nums">
                                                        ↓{cand.input.toFixed(2)} ↑{cand.output.toFixed(2)}
                                                    </span>
                                                    <span
                                                        className={cn(
                                                            'shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-semibold',
                                                            'bg-primary/10 text-primary'
                                                        )}
                                                    >
                                                        匹配
                                                    </span>
                                                </button>
                                            ))
                                        )}
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
import { useCallback, useMemo, useState } from 'react';
import { useLogs, useClearLogs } from '@/api/endpoints/log';
import { LogCard } from './Item';
import { Loader2, Trash2 } from 'lucide-react';
import { useTranslations } from 'use-intl';
import { VirtualizedGrid } from '@/components/common/VirtualizedGrid';
import { Button } from '@/components/ui/button';
import { toast } from 'sonner';

/**
 * 日志页面组件
 * - 初始加载 pageSize 条历史日志
 * - SSE 实时推送新日志
 * - 滚动自动加载更多
 * - 顶部清空按钮(调后端 /log/clear + 本地清缓存)
 */
export function Log() {
    const t = useTranslations('log');
    const { logs, hasMore, isLoading, isLoadingMore, loadMore, clear } = useLogs({ pageSize: 10 });
    const clearLogs = useClearLogs();
    const [isClearing, setIsClearing] = useState(false);

    const canLoadMore = hasMore && !isLoading && !isLoadingMore && logs.length > 0;
    const handleReachEnd = useCallback(() => {
        if (!canLoadMore) return;
        void loadMore();
    }, [canLoadMore, loadMore]);

    const handleClear = () => {
        if (!window.confirm(t('list.clearConfirm'))) return;
        setIsClearing(true);
        clearLogs.mutate(undefined, {
            onSuccess: () => {
                clear();
                toast.success(t('list.clearSuccess'));
                setIsClearing(false);
            },
            onError: () => {
                toast.error(t('list.clearFailed'));
                setIsClearing(false);
            },
        });
    };

    const footer = useMemo(() => {
        if (hasMore && (isLoading || isLoadingMore)) {
            return (
                <div className="flex justify-center py-4">
                    <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                </div>
            );
        }
        if (!hasMore && logs.length > 0) {
            return (
                <div className="flex justify-center py-4">
                    <span className="text-sm text-muted-foreground">{t('list.noMore')}</span>
                </div>
            );
        }
        return null;
    }, [hasMore, isLoading, isLoadingMore, logs.length, t]);

    return (
        <div className="flex h-full flex-col gap-3">
            <div className="flex items-center justify-end">
                <Button
                    variant="destructive"
                    size="sm"
                    onClick={handleClear}
                    disabled={isClearing}
                    className="rounded-xl"
                >
                    <Trash2 className="h-3.5 w-3.5 mr-1.5" />
                    {isClearing ? t('list.clearing') : t('list.clear')}
                </Button>
            </div>
            <div className="min-h-0 flex-1">
                <VirtualizedGrid
                    items={logs}
                    layout="list"
                    columns={{ default: 1 }}
                    estimateItemHeight={36}
                    overscan={8}
                    getItemKey={(log) => `log-${log.id}`}
                    renderItem={(log) => <LogCard log={log} />}
                    footer={footer}
                    onReachEnd={handleReachEnd}
                    reachEndEnabled={canLoadMore}
                    reachEndOffset={2}
                />
            </div>
        </div>
    );
}

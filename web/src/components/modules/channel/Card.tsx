import {
    MorphingDialog,
    MorphingDialogTrigger,
    MorphingDialogContainer,
    MorphingDialogContent,
} from '@/components/ui/morphing-dialog';
import { CheckCircle2, DollarSign, Layers, MessageSquare, XCircle } from 'lucide-react';
import { type StatsMetricsFormatted } from '@/api/endpoints/stats';
import { type Channel, useEnableChannel } from '@/api/endpoints/channel';
import { CardContent } from './CardContent';
import { useTranslations } from 'use-intl';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/animate-ui/components/animate/tooltip';
import { Switch } from '@/components/ui/switch';
import { Badge } from '@/components/ui/badge';
import { toast } from 'sonner';

export function Card({ channel, stats, layout = 'grid' }: { channel: Channel; stats: StatsMetricsFormatted; layout?: 'grid' | 'list' }) {
    const t = useTranslations('channel.card');
    const tForm = useTranslations('channel.form');
    const tMetrics = useTranslations('channel.detail.metrics');
    const enableChannel = useEnableChannel();
    const isListLayout = layout === 'list';

    const splitModels = (models: string) =>
        models
            .split(',')
            .map((item) => item.trim())
            .filter(Boolean);

    const modelCount = new Set([
        ...splitModels(channel.model),
        ...splitModels(channel.custom_model),
    ]).size;

    const handleEnableChange = (checked: boolean) => {
        enableChannel.mutate(
            { id: channel.id, enabled: checked },
            {
                onSuccess: () => {
                    toast.success(checked ? t('toast.enabled') : t('toast.disabled'));
                },
                onError: (error) => {
                    toast.error(error.message);
                },
            }
        );
    };

    return (
        <MorphingDialog>
            <MorphingDialogTrigger className="w-full">
                <article className={
                    isListLayout
                        ? 'flex flex-col rounded-3xl border border-border bg-card text-card-foreground px-3 py-1.5 transition-all duration-300'
                        : 'flex flex-col gap-4 rounded-3xl border border-border bg-card text-card-foreground p-4 transition-all duration-300'
                }>
                    {isListLayout ? (
                        <div className="flex w-full items-center gap-3">
                            <Tooltip side="top" sideOffset={10} align="center">
                                <TooltipTrigger asChild>
                                    <h3 className="min-w-0 flex-1 truncate text-left text-sm font-semibold">{channel.name}</h3>
                                </TooltipTrigger>
                                <TooltipContent key={channel.name}>{channel.name}</TooltipContent>
                            </Tooltip>
                            <Badge variant="secondary" className="shrink-0 text-[10px] font-normal text-muted-foreground">
                                {channel.type}
                            </Badge>
                            <span className="shrink-0 text-xs text-muted-foreground" title="模型数">
                                <Layers className="mr-1 inline size-3 -mt-0.5 text-primary" />
                                {modelCount}
                            </span>
                            <span className="shrink-0 text-xs text-muted-foreground" title="请求量">
                                <MessageSquare className="mr-1 inline size-3 -mt-0.5 text-primary" />
                                {stats.request_count.formatted.value}
                                {stats.request_count.formatted.unit && <span className="ml-0.5">{stats.request_count.formatted.unit}</span>}
                            </span>
                            <span className="shrink-0 text-xs" title="成功/失败">
                                <span className="text-emerald-500">✓{stats.request_success.formatted.value}</span>
                                <span className="ml-1.5 text-destructive">✗{stats.request_failed.formatted.value}</span>
                            </span>
                            <span className="shrink-0 text-xs text-muted-foreground" title="成本">
                                <DollarSign className="mr-0.5 inline size-3 -mt-0.5 text-primary" />
                                {stats.total_cost.formatted.value}
                                <span className="ml-0.5">{stats.total_cost.formatted.unit}</span>
                            </span>
                            <Switch
                                checked={channel.enabled}
                                onCheckedChange={handleEnableChange}
                                disabled={enableChannel.isPending}
                                onClick={(e) => e.stopPropagation()}
                                className="shrink-0"
                            />
                        </div>
                    ) : (
                        <>
                    <header className="relative flex items-center justify-between gap-2">
                        <Tooltip side="top" sideOffset={10} align="center">
                            <TooltipTrigger asChild>
                                <h3 className="text-lg font-bold truncate min-w-0">{channel.name}</h3>
                            </TooltipTrigger>
                            <TooltipContent key={channel.name}>{channel.name}</TooltipContent>
                        </Tooltip>
                        <Switch
                            checked={channel.enabled}
                            onCheckedChange={handleEnableChange}
                            disabled={enableChannel.isPending}
                            onClick={(e) => e.stopPropagation()}
                        />
                    </header>
                        <dl className="grid grid-cols-1 gap-3">
                            <div className="flex items-center justify-between rounded-2xl border border-border/70 bg-background/80 p-2">
                                <div className="flex items-center gap-3">
                                    <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                                        <MessageSquare className="h-5 w-5" />
                                    </span>
                                    <dt className="text-sm text-muted-foreground">{t('requestCount')}</dt>
                                </div>
                                <dd className="text-base">
                                    {stats.request_count.formatted.value}
                                    <span className="ml-1 text-xs text-muted-foreground">{stats.request_count.formatted.unit}</span>
                                </dd>
                            </div>

                            <div className="flex items-center justify-between rounded-2xl border border-border/70 bg-background/80 p-2">
                                <div className="flex items-center gap-3">
                                    <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                                        <DollarSign className="h-5 w-5" />
                                    </span>
                                    <dt className="text-sm text-muted-foreground">{t('totalCost')}</dt>
                                </div>
                                <dd className="text-base">
                                    {stats.total_cost.formatted.value}
                                    <span className="ml-1 text-xs text-muted-foreground">{stats.total_cost.formatted.unit}</span>
                                </dd>
                            </div>
                        </dl>
                        </>
                    )}
                </article>
            </MorphingDialogTrigger>

            <MorphingDialogContainer>
                <MorphingDialogContent className="w-full md:max-w-xl bg-card text-card-foreground px-4 py-2 rounded-3xl max-h-[90vh] overflow-y-auto">
                    <CardContent channel={channel} stats={stats} />
                </MorphingDialogContent>
            </MorphingDialogContainer>
        </MorphingDialog>
    );
}

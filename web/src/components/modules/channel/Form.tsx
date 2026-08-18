import { AutoGroupType, ChannelType, type Channel, useFetchModel, useChannelKey } from '@/api/endpoints/channel';
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { toast } from 'sonner';
import { useTranslations } from 'use-intl';
import { useEffect, useRef, useState } from 'react';
import { RefreshCw, X, Plus, Check, Search, Eye, EyeOff, KeyRound, Zap } from 'lucide-react';
import { ModelTestDialog } from './ModelTestDialog';

export interface ChannelFormData {
    name: string;
    type: ChannelType;
    base_url: string;
    key: string;
    custom_header: Channel['custom_header'];
    channel_proxy: string;
    proxy_pool: string;
    sticky: boolean;
    param_override: string;
    model: string;
    custom_model: string;
    enabled: boolean;
    proxy: boolean;
    max_concurrency: number;
    auto_sync: boolean;
    auto_group: AutoGroupType;
    match_regex: string;
    keys?: string[];
    key_strategy?: string;
}

export interface ChannelFormProps {
    formData: ChannelFormData;
    onFormDataChange: (data: ChannelFormData) => void;
    onSubmit: (event: React.FormEvent<HTMLFormElement>) => void;
    isPending: boolean;
    submitText: string;
    pendingText: string;
    onCancel?: () => void;
    cancelText?: string;
    idPrefix?: string;
    /** 编辑模式: 渠道 id, 用于眼睛切换时拉取完整 key(新建/复制时为空) */
    channelId?: number;
    /** 打开 Keys 管理弹窗(多 key 轮询) */
    onOpenKeysManager?: () => void;
}

import {
    Accordion,
    AccordionContent,
    AccordionItem,
    AccordionTrigger,
} from "@/components/ui/accordion";

export function ChannelForm({
    formData,
    onFormDataChange,
    onSubmit,
    isPending,
    submitText,
    pendingText,
    onCancel,
    cancelText,
    idPrefix = 'channel',
    channelId,
    onOpenKeysManager,
}: ChannelFormProps) {
    const t = useTranslations('channel.form');

    // Ensure the form always shows at least one custom Header row.
    useEffect(() => {
        if (!formData.custom_header || formData.custom_header.length === 0) {
            onFormDataChange({ ...formData, custom_header: [{ header_key: '', header_value: '' }] });
        }
    }, [formData, onFormDataChange]);

    const autoModels = formData.model
        ? formData.model.split(',').map((m) => m.trim()).filter(Boolean)
        : [];
    const customModels = formData.custom_model
        ? formData.custom_model.split(',').map((m) => m.trim()).filter(Boolean)
        : [];
    const [inputValue, setInputValue] = useState('');
    const inputRef = useRef<HTMLInputElement>(null);
    const [showKey, setShowKey] = useState(false);
    const keyBeforeRevealRef = useRef('');
    const channelKeyQuery = useChannelKey(channelId ?? 0);

    // 模型测试弹窗: 单个模型或全部
    const [testOpen, setTestOpen] = useState(false);
    const [testModels, setTestModels] = useState<string[]>([]);
    const [testPayload, setTestPayload] = useState<ReturnType<typeof buildTestPayload> | null>(null);

    // 获取明文 key(打码值先拉取): 拉取模型/测试模型都必须用明文
    const getPlainKey = async (): Promise<string | null> => {
        const key = formData.key.trim();
        if (!key.includes('****') || !channelId) return key || null;
        const res = await channelKeyQuery.refetch();
        return res.data ?? null;
    };

    // 测试弹窗的渠道配置 payload(与 fetch-model 同构 + 待测模型列表)
    const buildTestPayload = (key: string, models: string[]) => ({
        type: formData.type,
        base_url: formData.base_url.trim(),
        key,
        proxy: formData.proxy,
        channel_proxy: formData.channel_proxy?.trim() || null,
        proxy_pool: splitPool(formData.proxy_pool),
        sticky: formData.sticky,
        match_regex: formData.match_regex.trim() || null,
        custom_header: formData.custom_header?.filter((h) => h.header_key.trim()) || [],
        model_names: models,
        max_tokens: 5,
    });

    const handleTestModels = async (models: string[]) => {
        if (models.length === 0) return;
        const key = await getPlainKey();
        if (!key) {
            toast.error(t('modelRefreshFailed'), { description: '无法获取渠道密钥，请先点击密钥旁的眼睛显示' });
            return;
        }
        setTestPayload(buildTestPayload(key, models));
        setTestModels(models);
        setTestOpen(true);
    };

    // 眼睛切换: 打开时若 key 是打码值且可拉取明文, 用完整 key 替换显示; 关闭恢复打码(防误提交覆盖)
    const toggleKeyVisibility = () => {
        if (!showKey && channelId && formData.key.includes('****')) {
            keyBeforeRevealRef.current = formData.key;
            void channelKeyQuery.refetch().then((res) => {
                if (res.data) {
                    onFormDataChange({ ...formData, key: res.data });
                    setShowKey(true);
                }
            });
            return;
        }
        if (showKey && keyBeforeRevealRef.current) {
            onFormDataChange({ ...formData, key: keyBeforeRevealRef.current });
            keyBeforeRevealRef.current = '';
        }
        setShowKey((v) => !v);
    };

    const fetchModel = useFetchModel();

    // 模型候选框: 拉取后弹多选, 默认全不选, 确认后并入 model
    const [candidateModels, setCandidateModels] = useState<string[]>([]);
    const [selectedModels, setSelectedModels] = useState<Set<string>>(new Set());
    const [showCandidate, setShowCandidate] = useState(false);

    const updateModels = (nextAuto: string[], nextCustom: string[]) => {
        const model = nextAuto.join(',');
        const custom_model = nextCustom.join(',');
        if (formData.model === model && formData.custom_model === custom_model) return;
        onFormDataChange({ ...formData, model, custom_model });
    };

    const handleRefreshModels = async () => {
        if (!formData.base_url || !formData.key) return;
        let key = formData.key.trim();
        // 编辑已有渠道时 key 是打码值(前3****后4): 拉模型必须用明文, 先拉取再请求
        if (key.includes('****') && channelId) {
            const res = await channelKeyQuery.refetch();
            if (!res.data) {
                toast.error(t('modelRefreshFailed'), { description: '无法获取渠道密钥，请先点击密钥旁的眼睛显示' });
                return;
            }
            key = res.data;
        }
        fetchModel.mutate(
            {
                type: formData.type,
                base_url: formData.base_url.trim(),
                key,
                proxy: formData.proxy,
                channel_proxy: formData.channel_proxy?.trim() || null,
                proxy_pool: splitPool(formData.proxy_pool),
                sticky: formData.sticky,
                match_regex: formData.match_regex.trim() || null,
                custom_header: formData.custom_header?.filter((h) => h.header_key.trim()) || [],
            },
            {
                onSuccess: (data) => {
                    if (data && data.length > 0) {
                        // 弹候选框: 默认全不选
                        setCandidateModels(data);
                        setSelectedModels(new Set());
                        setShowCandidate(true);
                    } else {
                        toast.warning(t('modelRefreshEmpty'));
                    }
                },
                onError: (error) => {
                    const errorMessage = error instanceof Error ? error.message : String(error);
                    toast.error(t('modelRefreshFailed'), { description: errorMessage });
                },
            }
        );
    };

    // 确认候选: 勾选的并入 autoModels, 未勾选的丢弃
    const confirmCandidates = () => {
        const chosen = Array.from(selectedModels);
        const nextAuto = Array.from(new Set([...autoModels, ...chosen].map((m) => m.trim()).filter(Boolean)));
        updateModels(nextAuto, customModels);
        setShowCandidate(false);
        if (chosen.length > 0) toast.success(t('modelRefreshSuccess'));
    };

    const handleAddModel = (model: string) => {
        const trimmedModel = model.trim();
        if (trimmedModel && !customModels.includes(trimmedModel) && !autoModels.includes(trimmedModel)) {
            updateModels(autoModels, [...customModels, trimmedModel]);
        }
        setInputValue('');
    };

    const handleRemoveAutoModel = (model: string) => {
        updateModels(autoModels.filter(m => m !== model), customModels);
    };

    const handleRemoveCustomModel = (model: string) => {
        updateModels(autoModels, customModels.filter(m => m !== model));
    };

    const handleInputKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            if (inputValue.trim()) handleAddModel(inputValue);
        }
    };

    const handleAddHeader = () => {
        onFormDataChange({
            ...formData,
            custom_header: [...(formData.custom_header ?? []), { header_key: '', header_value: '' }],
        });
    };

    const handleUpdateHeader = (idx: number, patch: Partial<Channel['custom_header'][number]>) => {
        const next = (formData.custom_header ?? []).map((h, i) => (i === idx ? { ...h, ...patch } : h));
        onFormDataChange({ ...formData, custom_header: next });
    };

    const handleRemoveHeader = (idx: number) => {
        const curr = formData.custom_header ?? [];
        if (curr.length <= 1) return;
        onFormDataChange({ ...formData, custom_header: curr.filter((_, i) => i !== idx) });
    };

    return (
        <form onSubmit={onSubmit} className="space-y-4 px-1">
            {showCandidate && (
                <ModelCandidateDialog
                    models={candidateModels}
                    selected={selectedModels}
                    onToggle={(m) => {
                        const next = new Set(selectedModels);
                        if (next.has(m)) next.delete(m); else next.add(m);
                        setSelectedModels(next);
                    }}
                    onConfirm={confirmCandidates}
                    onCancel={() => setShowCandidate(false)}
                />
            )}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-2">
                    <label htmlFor={`${idPrefix}-name`} className="text-sm font-medium text-card-foreground">
                        {t('name')}
                    </label>
                    <Input
                        className='rounded-xl'
                        id={`${idPrefix}-name`}
                        type="text"
                        value={formData.name}
                        onChange={(event) => onFormDataChange({ ...formData, name: event.target.value })}
                        required
                    />
                </div>

                <div className="space-y-2">
                    <label htmlFor={`${idPrefix}-type`} className="text-sm font-medium text-card-foreground">
                        {t('type')}
                    </label>
                    <Select
                        value={String(formData.type)}
                        onValueChange={(value) => onFormDataChange({ ...formData, type: value as ChannelType })}
                    >
                        <SelectTrigger id={`${idPrefix}-type`} className="rounded-xl w-full border border-border px-4 py-2 text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                            <SelectValue />
                        </SelectTrigger>
                        <SelectContent className='rounded-xl'>
                            <SelectItem className='rounded-xl' value={String(ChannelType.OpenAIChat)}>{t('typeOpenAIChat')}</SelectItem>
                            <SelectItem className='rounded-xl' value={String(ChannelType.OpenAIResponse)}>{t('typeOpenAIResponse')}</SelectItem>
                            <SelectItem className='rounded-xl' value={String(ChannelType.Anthropic)}>{t('typeAnthropic')}</SelectItem>
                            <SelectItem className='rounded-xl' value={String(ChannelType.Gemini)}>{t('typeGemini')}</SelectItem>
                            <SelectItem className='rounded-xl' value={String(ChannelType.Mistral)}>{t('typeMistral')}</SelectItem>
                        </SelectContent>
                    </Select>
                </div>
            </div>

            <div className="space-y-2">
                <label htmlFor={`${idPrefix}-base-url`} className="text-sm font-medium text-card-foreground">
                    {t('baseUrl')}
                </label>
                <Input
                    id={`${idPrefix}-base-url`}
                    type="url"
                    value={formData.base_url}
                    onChange={(event) => onFormDataChange({ ...formData, base_url: event.target.value })}
                    placeholder={t('baseUrlUrl')}
                    required
                    className="rounded-xl"
                />
            </div>

            <div className="space-y-2">
                <label htmlFor={`${idPrefix}-key`} className="text-sm font-medium text-card-foreground">
                    {t('apiKey')}
                </label>
                <div className="relative">
                    <Input
                        id={`${idPrefix}-key`}
                        type="text"
                        value={formData.key}
                        onChange={(event) => onFormDataChange({ ...formData, key: event.target.value })}
                        placeholder={t('apiKey')}
                        required
                        className="rounded-xl pr-10"
                    />
                    <button
                        type="button"
                        onClick={toggleKeyVisibility}
                        className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                        tabIndex={-1}
                        aria-label={showKey ? t('hideKey') : t('showKey')}
                    >
                        {showKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </button>
                </div>
            </div>

            <div className="space-y-2">
                <label className="text-sm font-medium text-card-foreground">Keys（多 key 轮询）</label>
                <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={onOpenKeysManager}
                    className="w-full rounded-xl"
                >
                    <KeyRound className="h-3.5 w-3.5 mr-1.5" />
                    管理 Keys ({formData.keys?.length ?? 0})
                </Button>
            </div>

            <div className="space-y-2">
                <div className="flex items-center justify-between">
                    <label className="text-sm font-medium text-card-foreground">{t('model')}</label>
                    <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={handleRefreshModels}
                        disabled={!formData.base_url || !formData.key || fetchModel.isPending}
                        className="h-6 px-2 text-xs text-muted-foreground/50 hover:text-muted-foreground hover:bg-transparent"
                    >
                        <RefreshCw className={`h-3 w-3 mr-1 ${fetchModel.isPending ? 'animate-spin' : ''}`} />
                        {t('modelRefresh')}
                    </Button>
                </div>
                <input type="hidden" value={formData.model} required />

                <div className="relative">
                    <Input
                        ref={inputRef}
                        id={`${idPrefix}-model-custom`}
                        type="text"
                        value={inputValue}
                        onChange={(e) => setInputValue(e.target.value)}
                        onKeyDown={handleInputKeyDown}
                        placeholder={t('modelCustomPlaceholder')}
                        className="pr-10 rounded-xl"
                    />
                    {inputValue.trim() && !customModels.includes(inputValue.trim()) && !autoModels.includes(inputValue.trim()) && (
                        <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            onClick={() => handleAddModel(inputValue)}
                            className="absolute rounded-lg right-1 top-1/2 -translate-y-1/2 h-7 w-7 p-0 text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
                            title={t('modelAdd')}
                        >
                            <Plus className="size-4" />
                        </Button>
                    )}
                </div>

                <div className="space-y-2">
                    <div className="flex items-center justify-between">
                        <label className="text-xs font-medium text-card-foreground">
                            {t('modelSelected')} {(autoModels.length + customModels.length) > 0 && `(${autoModels.length + customModels.length})`}
                        </label>
                        {(autoModels.length + customModels.length) > 0 && (
                            <div className="flex items-center gap-1">
                                <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => handleTestModels([...autoModels, ...customModels])}
                                    className="h-6 px-2 text-xs text-primary hover:text-primary hover:bg-primary/10"
                                    title={t('modelTestAll')}
                                >
                                    <Zap className="size-3 mr-1" />
                                    {t('modelTestAll')}
                                </Button>
                                <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => {
                                        updateModels([], []);
                                    }}
                                    className="h-6 px-2 text-xs text-muted-foreground/50 hover:text-muted-foreground hover:bg-transparent"
                                >
                                    {t('modelClearAll')}
                                </Button>
                            </div>
                        )}
                    </div>
                    <div className="rounded-xl border border-border bg-muted/30 p-2.5 max-h-40 min-h-12 overflow-y-auto">
                        {(autoModels.length + customModels.length) > 0 ? (
                            <div className="flex flex-wrap gap-1.5">
                                {autoModels.map((model) => (
                                    <Badge key={model} variant="secondary" className="bg-muted hover:bg-muted/80">
                                        {model}
                                        <button
                                            type="button"
                                            onClick={() => handleTestModels([model])}
                                            className="ml-1 rounded-sm opacity-70 hover:opacity-100 hover:text-primary focus:outline-none focus:ring-1 focus:ring-ring"
                                            title={t('modelTest')}
                                        >
                                            <Zap className="h-3 w-3" />
                                        </button>
                                        <button
                                            type="button"
                                            onClick={() => handleRemoveAutoModel(model)}
                                            className="ml-0.5 rounded-sm opacity-70 hover:opacity-100 focus:outline-none focus:ring-1 focus:ring-ring"
                                        >
                                            <X className="h-3 w-3" />
                                        </button>
                                    </Badge>
                                ))}
                                {customModels.map((model) => (
                                    <Badge key={model} className="bg-primary hover:bg-primary/90">
                                        {model}
                                        <button
                                            type="button"
                                            onClick={() => handleTestModels([model])}
                                            className="ml-1 rounded-sm opacity-70 hover:opacity-100 hover:text-primary-foreground focus:outline-none focus:ring-1 focus:ring-ring"
                                            title={t('modelTest')}
                                        >
                                            <Zap className="h-3 w-3" />
                                        </button>
                                        <button
                                            type="button"
                                            onClick={() => handleRemoveCustomModel(model)}
                                            className="ml-0.5 rounded-sm opacity-70 hover:opacity-100 focus:outline-none focus:ring-1 focus:ring-ring"
                                        >
                                            <X className="h-3 w-3" />
                                        </button>
                                    </Badge>
                                ))}
                            </div>
                        ) : (
                            <div className="flex items-center justify-center h-8 text-xs text-muted-foreground">
                                {t('modelNoSelected')}
                            </div>
                        )}
                    </div>
                </div>
            </div>

            <Accordion type="single" collapsible className="w-full border rounded-xl bg-card">
                <AccordionItem value="advanced" className="border-none">
                    <AccordionTrigger className="text-sm font-medium text-card-foreground py-3 px-4 hover:no-underline hover:bg-muted/30 rounded-xl transition-colors">
                        {t('advanced')}
                    </AccordionTrigger>
                    <AccordionContent className="pt-4 px-4 pb-4 space-y-4 border-t">
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                            <div className="space-y-2">
                                <label htmlFor={`${idPrefix}-auto-group`} className="text-sm font-medium text-card-foreground">
                                    {t('autoGroup')}
                                </label>
                                <Select
                                    value={String(formData.auto_group)}
                                    onValueChange={(value) => onFormDataChange({ ...formData, auto_group: Number(value) as AutoGroupType })}
                                >
                                    <SelectTrigger id={`${idPrefix}-auto-group`} className="rounded-xl w-full border border-border px-4 py-2 text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                                        <SelectValue />
                                    </SelectTrigger>
                                    <SelectContent className='rounded-xl'>
                                        <SelectItem className='rounded-xl' value={String(AutoGroupType.None)}>{t('autoGroupNone')}</SelectItem>
                                        <SelectItem className='rounded-xl' value={String(AutoGroupType.Fuzzy)}>{t('autoGroupFuzzy')}</SelectItem>
                                        <SelectItem className='rounded-xl' value={String(AutoGroupType.Exact)}>{t('autoGroupExact')}</SelectItem>
                                        <SelectItem className='rounded-xl' value={String(AutoGroupType.Regex)}>{t('autoGroupRegex')}</SelectItem>
                                    </SelectContent>
                                </Select>
                            </div>

                            <div className="space-y-2">
                                <label htmlFor={`${idPrefix}-channel-proxy`} className="text-sm font-medium text-card-foreground">
                                    {t('channelProxy')}
                                </label>
                                <Input
                                    id={`${idPrefix}-channel-proxy`}
                                    type="text"
                                    value={formData.channel_proxy}
                                    onChange={(e) => onFormDataChange({ ...formData, channel_proxy: e.target.value })}
                                    placeholder={t('channelProxyPlaceholder')}
                                    className="rounded-xl"
                                />
                            </div>

                            <div className="space-y-2">
                                <label htmlFor={`${idPrefix}-proxy-pool`} className="text-sm font-medium text-card-foreground">
                                    {t('proxyPool')}
                                </label>
                                <Input
                                    id={`${idPrefix}-proxy-pool`}
                                    type="text"
                                    value={formData.proxy_pool}
                                    onChange={(e) => onFormDataChange({ ...formData, proxy_pool: e.target.value })}
                                    placeholder={t('proxyPoolPlaceholder')}
                                    className="rounded-xl"
                                />
                                <div className="flex items-center justify-between pt-1">
                                    <label className="flex items-center gap-2 cursor-pointer">
                                        <Switch
                                            checked={formData.sticky}
                                            onCheckedChange={(checked) => onFormDataChange({ ...formData, sticky: checked })}
                                        />
                                        <span className="text-sm text-card-foreground">{t('sticky')}</span>
                                    </label>
                                </div>
                            </div>
                        </div>

                        <div className="space-y-2">
                            <div className="flex items-center justify-between">
                                <label className="text-sm font-medium text-card-foreground">
                                    {t('customHeader')} {formData.custom_header.length > 0 ? `(${formData.custom_header.length})` : ''}
                                </label>
                                <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    onClick={handleAddHeader}
                                    className="h-6 px-2 text-xs text-muted-foreground/70 hover:text-muted-foreground hover:bg-transparent"
                                >
                                    <Plus className="h-3 w-3 mr-1" />
                                    {t('customHeaderAdd')}
                                </Button>
                            </div>
                            <div className="space-y-2">
                                {(formData.custom_header ?? []).map((h, idx) => (
                                    <div key={`hdr-${idx}`} className="flex items-center gap-2">
                                        <Input
                                            type="text"
                                            value={h.header_key}
                                            onChange={(e) => handleUpdateHeader(idx, { header_key: e.target.value })}
                                            placeholder={t('customHeaderKey')}
                                            className="rounded-xl flex-1"
                                        />
                                        <Input
                                            type="text"
                                            value={h.header_value}
                                            onChange={(e) => handleUpdateHeader(idx, { header_value: e.target.value })}
                                            placeholder={t('customHeaderValue')}
                                            className="rounded-xl flex-1"
                                        />
                                        <Button
                                            type="button"
                                            variant="ghost"
                                            size="sm"
                                            onClick={() => handleRemoveHeader(idx)}
                                            disabled={(formData.custom_header ?? []).length <= 1}
                                            className="h-8 w-8 p-0 rounded-xl text-muted-foreground hover:text-destructive hover:bg-transparent disabled:opacity-40"
                                            title="Remove"
                                        >
                                            <X className="h-4 w-4" />
                                        </Button>
                                    </div>
                                ))}
                            </div>
                        </div>

                        <div className="space-y-2">
                            <label htmlFor={`${idPrefix}-match-regex`} className="text-sm font-medium text-card-foreground">
                                {t('matchRegex')}
                            </label>
                            <Input
                                id={`${idPrefix}-match-regex`}
                                type="text"
                                value={formData.match_regex}
                                onChange={(e) => onFormDataChange({ ...formData, match_regex: e.target.value })}
                                placeholder={t('matchRegexPlaceholder')}
                                className="rounded-xl"
                            />
                        </div>

                        <div className="space-y-2">
                            <label htmlFor={`${idPrefix}-max-concurrency`} className="text-sm font-medium text-card-foreground">
                                {t('maxConcurrency')}
                            </label>
                            <Input
                                id={`${idPrefix}-max-concurrency`}
                                type="number"
                                min={0}
                                value={formData.max_concurrency}
                                onChange={(e) => onFormDataChange({ ...formData, max_concurrency: Number(e.target.value) || 0 })}
                                placeholder={t('maxConcurrencyPlaceholder')}
                                className="rounded-xl"
                            />
                        </div>

                        <div className="space-y-2">
                            <label htmlFor={`${idPrefix}-param-override`} className="text-sm font-medium text-card-foreground">
                                {t('paramOverride')}
                            </label>
                            <textarea
                                id={`${idPrefix}-param-override`}
                                value={formData.param_override}
                                onChange={(e) => onFormDataChange({ ...formData, param_override: e.target.value })}
                                placeholder={t('paramOverridePlaceholder')}
                                className="min-h-28 w-full rounded-xl border border-border bg-background px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            />
                        </div>
                    </AccordionContent>
                </AccordionItem>
            </Accordion>

            <div className="flex flex-wrap items-center justify-between gap-4 p-4 rounded-xl bg-muted/20 border border-border/50">
                <label className="flex items-center gap-2 cursor-pointer">
                    <Switch
                        checked={formData.enabled}
                        onCheckedChange={(checked) => onFormDataChange({ ...formData, enabled: checked })}
                    />
                    <span className="text-sm font-medium text-card-foreground">{t('enabled')}</span>
                </label>
                <div className="flex items-center gap-6">
                    <label className="flex items-center gap-2 cursor-pointer">
                        <Switch
                            checked={formData.proxy}
                            onCheckedChange={(checked) => onFormDataChange({ ...formData, proxy: checked })}
                        />
                        <span className="text-sm text-card-foreground">{t('proxy')}</span>
                    </label>
                    <label className="flex items-center gap-2 cursor-pointer">
                        <Switch
                            checked={formData.auto_sync}
                            onCheckedChange={(checked) => onFormDataChange({ ...formData, auto_sync: checked })}
                        />
                        <span className="text-sm text-card-foreground">{t('autoSync')}</span>
                    </label>
                </div>
            </div>

            <div className={`flex flex-col gap-3 pt-2 ${onCancel ? 'sm:flex-row' : ''}`}>
                {onCancel && cancelText && (
                    <Button
                        type="button"
                        variant="secondary"
                        onClick={onCancel}
                        className="w-full sm:flex-1 rounded-2xl h-12"
                    >
                        {cancelText}
                    </Button>
                )}
                <Button
                    type="submit"
                    disabled={isPending}
                    className="w-full sm:flex-1 rounded-2xl h-12"
                >
                    {isPending ? pendingText : submitText}
                </Button>
            </div>

            {testOpen && testPayload && (
                <ModelTestDialog
                    models={testModels}
                    testPayload={testPayload}
                    onClose={() => setTestOpen(false)}
                />
            )}
        </form>
    );
}

// 模型候选选择框: 拉取模型后弹出, checkbox 多选, 默认全不选。
export function ModelCandidateDialog({
    models,
    selected,
    onToggle,
    onConfirm,
    onCancel,
}: {
    models: string[];
    selected: Set<string>;
    onToggle: (m: string) => void;
    onConfirm: () => void;
    onCancel: () => void;
}) {
    const [search, setSearch] = useState('');
    const filtered = search.trim()
        ? models.filter((m) => m.toLowerCase().includes(search.trim().toLowerCase()))
        : models;

    return (
        <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 p-4">
            <div className="w-full max-w-md rounded-2xl border border-border bg-card p-5 shadow-2xl">
                <h3 className="mb-1 text-base font-bold text-card-foreground">选择要添加的模型</h3>
                <p className="mb-3 text-xs text-muted-foreground">
                    共 {models.length} 个模型，勾选后确认（默认全不选）
                </p>
                <div className="relative mb-3">
                    <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                    <Input
                        type="text"
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                        placeholder="搜索模型..."
                        className="rounded-xl pl-9"
                    />
                </div>
                <div className="max-h-72 overflow-y-auto rounded-xl border border-border bg-muted/30 p-2">
                    {filtered.length === 0 ? (
                        <div className="py-6 text-center text-xs text-muted-foreground">无匹配模型</div>
                    ) : (
                        filtered.map((m) => (
                            <label
                                key={m}
                                className="flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-sm hover:bg-muted"
                            >
                                <input
                                    type="checkbox"
                                    checked={selected.has(m)}
                                    onChange={() => onToggle(m)}
                                    className="accent-primary"
                                />
                                <span className="truncate font-mono text-foreground">{m}</span>
                            </label>
                        ))
                    )}
                </div>
                <div className="mt-4 flex justify-end gap-2">
                    <Button type="button" variant="ghost" size="sm" onClick={onCancel} className="rounded-xl">
                        <X className="h-3.5 w-3.5 mr-1" />取消
                    </Button>
                    <Button type="button" size="sm" onClick={onConfirm} className="rounded-xl">
                        <Check className="h-3.5 w-3.5 mr-1" />确认添加 ({selected.size})
                    </Button>
                </div>
            </div>
        </div>
    );
}

// splitPool 把逗号/换行分隔的代理池文本转成数组。
export function splitPool(text: string): string[] {
    return text
        .split(/[\n,]/)
        .map((s) => s.trim())
        .filter(Boolean);
}

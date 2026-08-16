import { useTranslations } from 'use-intl';
import { Info, Tag, Github, RefreshCw } from 'lucide-react';
import { APP_VERSION, GITHUB_REPO } from '@/lib/info';
import { Button } from '@/components/ui/button';
import { isOctopusCacheName, isFontCacheName, SW_MESSAGE_TYPE } from '@/lib/sw';

export function SettingInfo() {
    const t = useTranslations('setting');

    // 前端版本与后端当前版本不一致 → 浏览器缓存问题
    const clearCacheAndReload = async () => {
        // 通知 Service Worker 清理缓存
        if ('serviceWorker' in navigator && navigator.serviceWorker.controller) {
            navigator.serviceWorker.controller.postMessage({ type: SW_MESSAGE_TYPE.CLEAR_CACHE });
        }
        // 同时也从主线程清理（双保险），但保留字体缓存
        if ('caches' in window) {
            const names = await caches.keys();
            await Promise.all(
                names
                    .filter((name) => isOctopusCacheName(name) && !isFontCacheName(name))
                    .map((name) => caches.delete(name))
            );
        }
        // 注销当前 SW，下次加载会重新注册
        if ('serviceWorker' in navigator) {
            const registrations = await navigator.serviceWorker.getRegistrations();
            await Promise.all(registrations.map((reg) => reg.unregister()));
        }
        // 强制刷新（跳过缓存）
        window.location.reload();
    };

    return (
        <div className="rounded-3xl border border-border bg-card p-6 space-y-5">
            <h2 className="text-lg font-bold text-card-foreground flex items-center gap-2">
                <Info className="h-5 w-5" />
                {t('info.title')}
            </h2>
            {/* GitHub 仓库 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <Github className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('info.github')}</span>
                </div>
                <a
                    href={GITHUB_REPO}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-sm text-primary hover:underline"
                >
                    {GITHUB_REPO.replace('https://github.com/', '')}
                </a>
            </div>
            {/* 当前版本 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <Tag className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('info.currentVersion')}</span>
                </div>
                <code className="text-sm font-mono text-muted-foreground">
                    {APP_VERSION}
                </code>
            </div>

            {/* 强制刷新缓存（无版本更新逻辑，自维护 mod 不需要） */}
            <div className="flex justify-end">
                <Button
                    variant="outline"
                    size="sm"
                    onClick={clearCacheAndReload}
                    className="rounded-xl"
                >
                    <RefreshCw className="h-3.5 w-3.5 mr-1.5" />
                    {t('info.forceRefresh')}
                </Button>
            </div>
        </div>
    );
}

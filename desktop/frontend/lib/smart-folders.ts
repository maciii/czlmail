"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { t } from "@/lib/i18n";
import { Events, api, onEvent } from "@/lib/api";
import type { main } from "@/wailsjs/go/models";

/**
 * 筛选文件夹: 自己组合条件 (未读、待办旗标、标签、有附件、发件人、账号、范围) 的跨账号视图,
 * 侧栏「全部账号」里排在「未读」下面。定义经个人网盘的 smart-folders.json 同步, 见后端 smart_folders.go。
 */
export type SmartFolder = main.SmartFolder;
/** 本地缓存里的邮件数与其中的未读数; accounts 按本机账号 id 分开数, 没有的账号不出现 (后端 SmartFolderCount)。 */
export interface SmartFolderCount {
  total: number;
  unread: number;
  accounts?: Record<string, { total: number; unread: number }>;
}

/**
 * 统一视图后面接 "@" + 本机账号 id 时只看那一个账号 (「未读」与筛选文件夹按账号展开的行), 与后端 splitViewAccount 一致。
 */
export const viewForAccount = (view: string, accountId: string) => `${view}@${accountId}`;

/** 拆出统一视图与限定的账号 (没有限定时为空串)。 */
export function splitViewAccount(view: string): { base: string; account: string } {
  const i = view.indexOf("@");
  return i < 0 ? { base: view, account: "" } : { base: view.slice(0, i), account: view.slice(i + 1) };
}

/** 统一视图里筛选文件夹的 view 前缀, 与后端 smartViewPrefix 一致。 */
export const SMART_VIEW_PREFIX = "smart:";

export const smartView = (id: string) => SMART_VIEW_PREFIX + id;

/** view 是筛选文件夹时返回它的 id, 否则为 null。 */
export function smartIdOf(view: string): string | null {
  const { base } = splitViewAccount(view);
  return base.startsWith(SMART_VIEW_PREFIX) ? base.slice(SMART_VIEW_PREFIX.length) : null;
}

/**
 * 不对应某一类文件夹的统一视图 (「未读」、筛选文件夹, 以及它们按账号展开的行):
 * 列表里的邮件来自各种文件夹, 没有单一的「当前文件夹」。
 */
export function isMixedView(view: string): boolean {
  const { base, account } = splitViewAccount(view);
  return base === "unread" || base.startsWith(SMART_VIEW_PREFIX) || account !== "";
}

export type SmartFlag = "" | "flagged" | "completed";
export type SmartScope = "" | "inbox" | "all";

export function emptySmartFolder(): SmartFolder {
  return {
    id: "",
    name: "",
    unread: false,
    flag: "",
    tags: [],
    attachment: false,
    from: "",
    accounts: [],
    accountsElsewhere: 0,
    scope: "",
  } as SmartFolder;
}

/** 在哪些文件夹里找, 下拉框的选项。 */
export function smartScopeOptions(): [SmartScope, string][] {
  return [
    ["", t("除已发送、草稿、垃圾邮件与已删除外的文件夹（同「未读」）")],
    ["inbox", t("只在收件箱")],
    ["all", t("除垃圾邮件与已删除外的全部文件夹")],
  ];
}

/** 筛选文件夹的条件写成一行, 设置页的列表与侧栏的提示用。 */
export function smartFolderSummary(f: SmartFolder, tagName: (kw: string) => string, accountName: (id: string) => string): string {
  const parts: string[] = [];
  if (f.unread) parts.push(t("未读"));
  if (f.flag === "flagged") parts.push(t("带待办旗标"));
  if (f.flag === "completed") parts.push(t("待办已完成"));
  if (f.tags?.length) parts.push(t("标签：{0}", f.tags.map(tagName).join(t("或"))));
  if (f.attachment) parts.push(t("有附件"));
  if (f.from) parts.push(t("发件人包含「{0}」", f.from));
  const accounts = (f.accounts ?? []).map(accountName);
  if (f.accountsElsewhere) accounts.push(t("其它设备上的 {0} 个账号", f.accountsElsewhere));
  if (accounts.length) parts.push(accounts.join(", "));
  if (f.scope === "inbox") parts.push(t("只在收件箱"));
  if (f.scope === "all") parts.push(t("全部文件夹"));
  return parts.length ? parts.join(" · ") : t("没有条件");
}

/** 读筛选文件夹; 本机修改与别的设备同步过来时 (都发 settings:changed) 跟着更新。loaded 为读到过一次。 */
export function useSmartFolders(): { folders: SmartFolder[]; loaded: boolean } {
  const [state, setState] = useState<{ folders: SmartFolder[]; loaded: boolean }>({ folders: [], loaded: false });
  useEffect(() => {
    const load = () =>
      api
        .getSmartFolders()
        .then((l) => setState({ folders: l ?? [], loaded: true }))
        .catch(() => {});
    load();
    const offSettings = onEvent(Events.settingsChanged, load);
    // 登录增减时本机账号 id 与服务器键的对应跟着变。
    const offAccounts = onEvent(Events.accountsChanged, load);
    return () => {
      offSettings();
      offAccounts();
    };
  }, []);
  return state;
}

/**
 * 各筛选文件夹本地缓存里的邮件数与未读数。定义变了很快重数; 文件夹未读数 (deps) 变化与邮件变化事件
 * (只改了旗标、标签的邮件不一定让未读数变) 合在一起, 1 秒内只数一次: 同步一轮会连着发好几次。
 */
export function useSmartFolderCounts(folders: SmartFolder[], deps: unknown): Record<string, SmartFolderCount> {
  const [counts, setCounts] = useState<Record<string, SmartFolderCount>>({});
  const active = folders.length > 0;
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const schedule = useCallback((delay: number) => {
    clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      api
        .smartFolderCounts()
        .then((c) => setCounts((c ?? {}) as Record<string, SmartFolderCount>))
        .catch(() => {});
    }, delay);
  }, []);
  useEffect(() => {
    if (active) schedule(300);
  }, [active, folders, schedule]);
  useEffect(() => {
    if (active) schedule(1000);
  }, [active, deps, schedule]);
  useEffect(() => {
    if (!active) return;
    const off = onEvent(Events.mailChanged, () => schedule(1000));
    return () => {
      clearTimeout(timer.current);
      off();
    };
  }, [active, schedule]);
  return counts;
}

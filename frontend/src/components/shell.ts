import { esc } from '../shared/format';
import { browserPreview } from '../bridge';

export function navButton(id: string, icon: string, label: string, page = '') {
  return `<button class="side-item" data-page="${esc(id)}" aria-current="${page === id ? 'page' : 'false'}" title="${esc(label)}"><span aria-hidden="true" class="nav-icon">${icon}</span><span class="nav-label">${esc(label)}</span></button>`;
}

export function shell() {
  return `<div class="workspace">
    <aside class="sidebar" aria-label="워크스페이스 메뉴">
      <div class="brand"><span class="brand-mark" aria-hidden="true">◈</span><div><strong>Atlas</strong><small>운영 워크스페이스</small></div></div>
      <div class="side-label">WORKSPACE</div>
      <nav aria-label="주 메뉴">${navButton('home', '◫', '운영 현황')}${navButton('dashboard', '▦', '내 대시보드')}</nav>
      <div class="side-label group-label">탐색</div>
      <nav aria-label="데이터 탐색">${navButton('overview', '◷', '수집 상세')}${navButton('history', '≡', '빌드 이력')}${navButton('metrics', '⌁', '지표 탐색')}${navButton('logs', '☷', '로그 탐색')}${navButton('traces', '⌘', '트레이스 탐색')}${navButton('api', '⌗', 'API 쿼리')}</nav>
      <div class="side-label plugins-label">플러그인</div><nav id="plugin-nav" aria-label="플러그인 대시보드"></nav>
      <div class="side-bottom"><nav aria-label="설정">${navButton('connections', '⚙', '연결 설정')}${navButton('plugins', '⊞', '플러그인')}</nav>
        <div class="workspace-name"><span aria-hidden="true">◉</span><div><b>내 워크스페이스</b><small id="connection-count"></small></div></div>
      </div>
    </aside>
    <main class="main"><header class="topbar"><span>워크스페이스 <span class="crumb">/</span> <span id="page-title">운영 현황</span></span><div><span class="environment"><i></i>${browserPreview ? 'DEMO DATA' : 'LOCAL'}</span><button class="icon-button" id="refresh" aria-label="새로고침" title="새로고침">↻</button></div></header>
      <div id="banner"></div><div id="page" class="content"></div>
    </main>
  </div>`;
}

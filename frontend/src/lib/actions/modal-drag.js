/**
 * modalDrag — Svelte action that makes a dialog draggable by a handle row.
 *
 * Usage:
 *   <div class="modal" use:modalDrag=".settings-head">
 *
 * The handle row is dragged with the pointer; the movement is applied as a
 * `translate()` on the dialog node (so flex centering in the backdrop is not
 * disturbed). Movement is clamped so the dialog can never be dragged fully
 * off-screen. Because dialogs here are conditionally rendered ({#if open}),
 * the translate resets naturally on close/reopen.
 *
 * Safety rails:
 *  - pointerdown on interactive elements inside the handle (buttons, inputs,
 *    tabs, summaries) never starts a drag;
 *  - if the pointer actually moved, the trailing `click` is suppressed in the
 *    capture phase, so backdrop click-to-close handlers don't dismiss the
 *    dialog mid-drag (mouseup lands on the backdrop after a drag);
 *  - left button only; text selection is prevented during the drag.
 *
 * Pair with CSS `resize: both` on the dialog for free resizing.
 */
export function modalDrag(node, handleSelector = '') {
	let dragging = false;
	let moved = false;
	let startX = 0;
	let startY = 0;
	let origX = 0;
	let origY = 0;
	let releaseClickGuard = null;

	function currentTranslate() {
		const t = getComputedStyle(node).transform;
		if (!t || t === 'none') return { x: 0, y: 0 };
		const m = new DOMMatrixReadOnly(t);
		return { x: m.m41, y: m.m42 };
	}

	function onPointerDown(e) {
		if (e.button !== 0 || dragging) return;
		const handle = handleSelector ? node.querySelector(handleSelector) : node;
		if (!handle || !handle.contains(e.target)) return;
		if (e.target.closest('button, input, select, textarea, a, [role="tab"], summary')) return;
		e.preventDefault();
		dragging = true;
		moved = false;
		startX = e.clientX;
		startY = e.clientY;
		const tr = currentTranslate();
		origX = tr.x;
		origY = tr.y;
		window.addEventListener('pointermove', onPointerMove);
		window.addEventListener('pointerup', onPointerUp);
		window.addEventListener('pointercancel', onPointerUp);
	}

	function onPointerMove(e) {
		if (!dragging) return;
		const dx = e.clientX - startX;
		const dy = e.clientY - startY;
		if (!moved && Math.abs(dx) + Math.abs(dy) < 3) return;
		moved = true;
		const rect = node.getBoundingClientRect();
		// Keep at least ~60px of the dialog inside the viewport.
		const nx = Math.min(window.innerWidth - 60, Math.max(60 - rect.width, origX + dx));
		const ny = Math.min(window.innerHeight - 48, Math.max(4, origY + dy));
		node.style.transform = `translate(${nx}px, ${ny}px)`;
	}

	function onPointerUp() {
		if (!dragging) return;
		dragging = false;
		window.removeEventListener('pointermove', onPointerMove);
		window.removeEventListener('pointerup', onPointerUp);
		window.removeEventListener('pointercancel', onPointerUp);
		if (moved) {
			// Swallow the trailing click (capture phase, one shot) so a drag that
			// ends over the backdrop cannot trigger click-to-close.
			const guard = (ev) => {
				ev.stopPropagation();
				ev.preventDefault();
			};
			window.addEventListener('click', guard, true);
			releaseClickGuard = setTimeout(() => {
				window.removeEventListener('click', guard, true);
				releaseClickGuard = null;
			}, 0);
		}
	}

	node.addEventListener('pointerdown', onPointerDown);

	return {
		destroy() {
			node.removeEventListener('pointerdown', onPointerDown);
			window.removeEventListener('pointermove', onPointerMove);
			window.removeEventListener('pointerup', onPointerUp);
			window.removeEventListener('pointercancel', onPointerUp);
			if (releaseClickGuard) clearTimeout(releaseClickGuard);
		},
	};
}

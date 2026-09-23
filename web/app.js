// ── StockFlow App ──
// Frontend com Autenticação e Controle de Permissões (Admin vs Funcionário)

const API_BASE = '';

// ── State ──
let currentUser = null;
let authToken = localStorage.getItem('stockflow_token') || null;
let allProducts = [];
let allUsers = [];

// ── Initialization ──
document.addEventListener('DOMContentLoaded', () => {
  if (authToken) {
    checkSession();
  } else {
    showLoginScreen();
  }
});

// ── API Helper with Token Header ──
async function apiFetch(endpoint, options = {}) {
  if (!['http:', 'https:'].includes(window.location.protocol)) {
    throw new Error('Abra o sistema em http://localhost:8081 com a API Go em execução. O login não funciona abrindo o arquivo HTML diretamente.');
  }

  const url = `${API_BASE}${endpoint}`;
  const headers = {
    'Content-Type': 'application/json',
    ...(authToken ? { 'Authorization': `Bearer ${authToken}` } : {}),
    ...options.headers,
  };

  const config = { ...options, headers };
  let response;
  try {
    response = await fetch(url, config);
  } catch (err) {
    throw new Error('Não foi possível conectar à API. Verifique se ela está em execução e acesse http://localhost:8081.');
  }

  if (response.status === 401 && endpoint !== '/login') {
    logout();
    throw new Error('Sessão expirada. Faça login novamente.');
  }

  if (!(response.headers.get('Content-Type') || '').toLowerCase().includes('application/json')) {
    throw new Error(`O servidor retornou uma resposta inesperada (HTTP ${response.status}). Acesse o sistema pela API Go em http://localhost:8081.`);
  }

  let data;
  try {
    data = await response.json();
  } catch (err) {
    throw new Error('A API retornou uma resposta JSON inválida. Verifique os logs do servidor.');
  }

  if (!response.ok) {
    throw new Error(data?.error || `Erro ${response.status}`);
  }

  return data;
}

// ── Session & Auth ──
async function checkSession() {
  try {
    currentUser = await apiFetch('/me');
    showAppScreen();
  } catch (err) {
    logout();
  }
}

async function handleLoginSubmit(event) {
  event.preventDefault();
  const email = document.getElementById('login-email').value;
  const password = document.getElementById('login-password').value;

  try {
    const data = await apiFetch('/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    });

    authToken = data.token;
    currentUser = data.user;
    localStorage.setItem('stockflow_token', authToken);

    showToast(`Bem-vindo, ${currentUser.name}!`, 'success');
    showAppScreen();
  } catch (err) {
    showToast(err.message, 'error');
  }
}

function logout() {
  authToken = null;
  currentUser = null;
  localStorage.removeItem('stockflow_token');
  showLoginScreen();
}

function showLoginScreen() {
  document.getElementById('login-screen').classList.add('active');
  document.getElementById('app-container').style.display = 'none';
}

function showAppScreen() {
  document.getElementById('login-screen').classList.remove('active');
  document.getElementById('app-container').style.display = 'block';

  // Render User Header Info
  document.getElementById('user-display-name').textContent = currentUser.name;
  document.getElementById('user-display-role').textContent = currentUser.role.toUpperCase();

  // Show/Hide Admin Navigation & Elements
  const isAdmin = currentUser.role === 'admin';
  document.getElementById('admin-nav').style.display = isAdmin ? 'flex' : 'none';

  document.querySelectorAll('.admin-only').forEach(el => {
    el.style.display = isAdmin ? 'inline-flex' : 'none';
  });

  // Default tab
  switchTab('products');
  loadDashboard();
  loadProducts();

  if (isAdmin) {
    loadUsers();
  }
}

// ── Navigation Tabs ──
function switchTab(tabName) {
  document.querySelectorAll('.tab-content').forEach(el => el.style.display = 'none');
  document.querySelectorAll('.nav-tab').forEach(el => el.classList.remove('active'));

  if (tabName === 'products') {
    document.getElementById('tab-products').style.display = 'block';
    document.getElementById('tab-products-btn').classList.add('active');
  } else if (tabName === 'users') {
    document.getElementById('tab-users').style.display = 'block';
    document.getElementById('tab-users-btn').classList.add('active');
    loadUsers();
  }
}

// ── Dashboard ──
async function loadDashboard() {
  try {
    const stats = await apiFetch('/dashboard');
    document.getElementById('stat-total-products').textContent = stats.total_products;
    document.getElementById('stat-total-items').textContent = stats.total_items;
    document.getElementById('stat-total-value').textContent = formatCurrency(stats.total_value_cents);
    document.getElementById('stat-low-stock').textContent = stats.low_stock_count;

    const alertCard = document.getElementById('stat-card-alerts');
    if (stats.low_stock_count > 0) {
      alertCard.classList.add('has-alerts');
    } else {
      alertCard.classList.remove('has-alerts');
    }
  } catch (err) {
    console.error('Erro ao carregar dashboard:', err);
  }
}

// ── Products ──
async function loadProducts() {
  try {
    allProducts = await apiFetch('/products');
    renderProducts(allProducts);
  } catch (err) {
    console.error('Erro ao carregar produtos:', err);
  }
}

function renderProducts(products) {
  const tbody = document.getElementById('products-tbody');
  const emptyState = document.getElementById('empty-state');
  const isAdmin = currentUser && currentUser.role === 'admin';

  if (products.length === 0) {
    tbody.innerHTML = '';
    emptyState.style.display = 'block';
    return;
  }

  emptyState.style.display = 'none';

  tbody.innerHTML = products.map((p) => {
    const stockStatus = getStockStatus(p);
    return `
      <tr>
        <td><span class="sku-tag">${escapeHtml(p.sku)}</span></td>
        <td>
          <div style="font-weight: 600;">${escapeHtml(p.name)}</div>
          ${p.description ? `<div style="font-size: 0.78rem; color: var(--text-muted); margin-top: 2px;">${escapeHtml(truncate(p.description, 60))}</div>` : ''}
        </td>
        <td><span class="price-display">${formatCurrency(p.price_cents)}</span></td>
        <td style="font-weight: 600;">${p.stock_quantity.toLocaleString('pt-BR')}</td>
        <td style="color: var(--text-muted);">${p.minimum_stock}</td>
        <td>
          <span class="stock-badge stock-badge--${stockStatus.class}">
            <span class="stock-badge__dot"></span>
            ${stockStatus.label}
          </span>
        </td>
        <td>
          <div class="actions-cell">
            <button class="action-btn action-btn--stock-add" title="Ajustar estoque (+/-)" onclick="openStockModal(${p.id}, '${escapeHtml(p.name)}', ${p.stock_quantity})">
              📊
            </button>
            ${isAdmin ? `
              <button class="action-btn action-btn--edit" title="Editar produto" onclick="openEditModal(${p.id})">✏️</button>
              <button class="action-btn action-btn--delete" title="Excluir produto" onclick="openDeleteModal(${p.id}, '${escapeHtml(p.name)}')">🗑️</button>
            ` : ''}
          </div>
        </td>
      </tr>
    `;
  }).join('');
}

function getStockStatus(product) {
  if (product.stock_quantity === 0) return { class: 'critical', label: 'Sem estoque' };
  if (product.minimum_stock > 0 && product.stock_quantity <= product.minimum_stock) return { class: 'low', label: 'Estoque baixo' };
  return { class: 'ok', label: 'Normal' };
}

function filterProducts() {
  const query = document.getElementById('search-field').value.toLowerCase().trim();
  if (!query) {
    renderProducts(allProducts);
    return;
  }
  const filtered = allProducts.filter(p =>
    p.name.toLowerCase().includes(query) ||
    p.sku.toLowerCase().includes(query) ||
    (p.description && p.description.toLowerCase().includes(query))
  );
  renderProducts(filtered);
}

// ── Create / Edit Product ──
function openCreateModal() {
  document.getElementById('modal-product-title').textContent = 'Novo Produto';
  document.getElementById('btn-submit-product').textContent = 'Criar Produto';
  document.getElementById('product-form').reset();
  document.getElementById('form-product-id').value = '';
  openModal('modal-product');
}

async function openEditModal(id) {
  try {
    const product = await apiFetch(`/products/${id}`);
    document.getElementById('modal-product-title').textContent = 'Editar Produto';
    document.getElementById('btn-submit-product').textContent = 'Salvar Alterações';
    document.getElementById('form-product-id').value = product.id;
    document.getElementById('form-sku').value = product.sku;
    document.getElementById('form-name').value = product.name;
    document.getElementById('form-description').value = product.description;
    document.getElementById('form-price').value = (product.price_cents / 100).toFixed(2);
    document.getElementById('form-stock').value = product.stock_quantity;
    document.getElementById('form-minimum').value = product.minimum_stock;
    openModal('modal-product');
  } catch (err) {
    showToast(err.message, 'error');
  }
}

async function handleProductSubmit(event) {
  event.preventDefault();

  const id = document.getElementById('form-product-id').value;
  const priceValue = parseFloat(document.getElementById('form-price').value);

  const body = {
    sku: document.getElementById('form-sku').value.trim(),
    name: document.getElementById('form-name').value.trim(),
    description: document.getElementById('form-description').value.trim(),
    price_cents: Math.round(priceValue * 100),
    stock_quantity: parseInt(document.getElementById('form-stock').value) || 0,
    minimum_stock: parseInt(document.getElementById('form-minimum').value) || 0,
  };

  try {
    if (id) {
      await apiFetch(`/products/${id}`, { method: 'PUT', body: JSON.stringify(body) });
      showToast('Produto atualizado com sucesso!', 'success');
    } else {
      await apiFetch('/products', { method: 'POST', body: JSON.stringify(body) });
      showToast('Produto criado com sucesso!', 'success');
    }
    closeModal('modal-product');
    await Promise.all([loadProducts(), loadDashboard()]);
  } catch (err) {
    showToast(err.message, 'error');
  }
}

// ── Stock Adjust Modal ──
function openStockModal(id, name, currentQty) {
  document.getElementById('stock-product-id').value = id;
  document.getElementById('stock-product-name').textContent = name;
  document.getElementById('stock-current-qty').textContent = currentQty;
  document.getElementById('stock-adjust-qty').value = 1;
  openModal('modal-stock');
}

function changeStockQty(delta) {
  const input = document.getElementById('stock-adjust-qty');
  let val = parseInt(input.value) || 0;
  val = Math.max(1, val + delta);
  input.value = val;
}

async function submitStockAdjust(direction) {
  const id = document.getElementById('stock-product-id').value;
  const qty = parseInt(document.getElementById('stock-adjust-qty').value) || 1;
  const quantity = qty * direction;

  try {
    await apiFetch(`/products/${id}/stock`, {
      method: 'PATCH',
      body: JSON.stringify({ quantity }),
    });
    const action = direction > 0 ? 'Entrada' : 'Saída';
    showToast(`${action} de ${qty} unidade(s) registrada!`, 'success');
    closeModal('modal-stock');
    await Promise.all([loadProducts(), loadDashboard()]);
  } catch (err) {
    showToast(err.message, 'error');
  }
}

// ── Delete Product Modal ──
function openDeleteModal(id, name) {
  document.getElementById('confirm-product-id').value = id;
  document.getElementById('confirm-product-name').textContent = name;
  openModal('modal-confirm');
}

async function confirmDelete() {
  const id = document.getElementById('confirm-product-id').value;
  try {
    await apiFetch(`/products/${id}`, { method: 'DELETE' });
    showToast('Produto removido com sucesso!', 'success');
    closeModal('modal-confirm');
    await Promise.all([loadProducts(), loadDashboard()]);
  } catch (err) {
    showToast(err.message, 'error');
  }
}

// ── Users / Employees Management (Admin Only) ──
async function loadUsers() {
  try {
    allUsers = await apiFetch('/users');
    renderUsers(allUsers);
  } catch (err) {
    console.error('Erro ao carregar usuários:', err);
  }
}

function renderUsers(users) {
  const tbody = document.getElementById('users-tbody');
  tbody.innerHTML = users.map(u => `
    <tr>
      <td>#${u.id}</td>
      <td style="font-weight: 600;">${escapeHtml(u.name)}</td>
      <td>${escapeHtml(u.email)}</td>
      <td>
        <span class="role-badge role-badge--${u.role}">
          ${u.role === 'admin' ? 'Administrador' : 'Funcionário'}
        </span>
      </td>
      <td>${new Date(u.created_at).toLocaleDateString('pt-BR')}</td>
      <td>
        ${u.id !== currentUser.id ? `
          <button class="action-btn action-btn--delete" title="Excluir usuário" onclick="deleteUser(${u.id}, '${escapeHtml(u.name)}')">🗑️</button>
        ` : '<span style="font-size:0.75rem; color:var(--text-muted);">(Você)</span>'}
      </td>
    </tr>
  `).join('');
}

function openUserModal() {
  document.getElementById('user-form').reset();
  openModal('modal-user');
}

async function handleUserSubmit(event) {
  event.preventDefault();

  const body = {
    name: document.getElementById('user-name').value.trim(),
    email: document.getElementById('user-email').value.trim(),
    password: document.getElementById('user-password').value,
    role: document.getElementById('user-role').value,
  };

  try {
    await apiFetch('/users', { method: 'POST', body: JSON.stringify(body) });
    showToast('Funcionário cadastrado com sucesso!', 'success');
    closeModal('modal-user');
    loadUsers();
  } catch (err) {
    showToast(err.message, 'error');
  }
}

async function deleteUser(id, name) {
  if (!confirm(`Deseja realmente remover o acesso de ${name}?`)) return;

  try {
    await apiFetch(`/users/${id}`, { method: 'DELETE' });
    showToast('Usuário removido!', 'success');
    loadUsers();
  } catch (err) {
    showToast(err.message, 'error');
  }
}

// ── Modal Helpers ──
function openModal(id) {
  const overlay = document.getElementById(id);
  overlay.classList.add('active');
  document.body.style.overflow = 'hidden';
}

function closeModal(id) {
  document.getElementById(id).classList.remove('active');
  document.body.style.overflow = '';
}

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') {
    document.querySelectorAll('.modal-overlay.active').forEach(overlay => {
      closeModal(overlay.id);
    });
  }
});

// ── Toast Notifications ──
function showToast(message, type = 'info') {
  const container = document.getElementById('toast-container');
  const icons = { success: '✅', error: '❌', info: 'ℹ️' };

  const toast = document.createElement('div');
  toast.className = `toast toast--${type}`;
  toast.innerHTML = `<span>${icons[type] || ''}</span> ${escapeHtml(message)}`;

  container.appendChild(toast);

  setTimeout(() => {
    toast.remove();
  }, 3500);
}

// ── Formatting Helpers ──
function formatCurrency(cents) {
  return (cents / 100).toLocaleString('pt-BR', { style: 'currency', currency: 'BRL' });
}

function truncate(str, max) {
  return str.length > max ? str.substring(0, max) + '…' : str;
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

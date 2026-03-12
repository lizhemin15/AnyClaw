using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Net.Http;
using System.Net.WebSockets;
using System.Text;
using System.Text.Json;
using System.Text.RegularExpressions;
using System.Threading;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Controls.Primitives;
using System.Windows.Documents;
using System.Windows.Input;
using System.Windows.Media;
using System.Windows.Media.Effects;
using System.Windows.Shapes;
using PathShape = System.Windows.Shapes.Path;
using Quicker.Public;

public static string Exec(IStepContext context)
{
    try
    {
        System.Net.ServicePointManager.ServerCertificateValidationCallback = (sender, cert, chain, err) => true;
        string apiBase = (context.GetVarValue("api_base") as string)?.Trim();
        if (string.IsNullOrEmpty(apiBase)) apiBase = "https://htwkumkjgrnz.sealosbja.site";
        if (!apiBase.StartsWith("http")) apiBase = "https://" + apiBase;

        string email = (context.GetVarValue("email") as string)?.Trim();
        string password = (context.GetVarValue("password") as string)?.Trim();
        string mode = ((context.GetVarValue("mode") as string)?.Trim() ?? "desktop").ToLowerInvariant();

        string token = LoadToken(apiBase);
        if (string.IsNullOrEmpty(token) && !string.IsNullOrEmpty(email) && !string.IsNullOrEmpty(password))
        {
            token = DoLogin(apiBase, email, password);
            if (!string.IsNullOrEmpty(token)) SaveToken(apiBase, token);
        }

        if (string.IsNullOrEmpty(token))
        {
            string finalToken = null;
            Application.Current.Dispatcher.Invoke(() =>
            {
                var loginWin = new LoginWindow(apiBase, email, password);
                if (loginWin.ShowDialog() == true)
                {
                    finalToken = loginWin.Token;
                    if (!string.IsNullOrEmpty(finalToken)) SaveToken(apiBase, finalToken);
                }
            });
            token = finalToken;
        }

        if (string.IsNullOrEmpty(token))
        {
            return "OK";
        }

        Application.Current.Dispatcher.Invoke(() =>
        {
            if (mode == "desktop")
            {
                var admin = new ClawAdminWindow(apiBase, token);
                admin.Show();
                var config = LoadDesktopConfig(apiBase);
                var instances = GetInstances(apiBase, token);
                for (var i = 0; i < config.Count; i++)
                {
                    var (id, schemeId, x, y) = config[i];
                    var inst = instances.FirstOrDefault(x => x.Id == id);
                    if (inst != null && inst.Status == "running")
                    {
                        var m = new ClawMascotWindow(apiBase, token, id, inst.Name, schemeId, i, x, y);
                        m.Show();
                    }
                }
            }
            else
            {
                var main = new AnyClawMainWindow(apiBase, token);
                main.ShowDialog();
            }
        });
        return "OK";
    }
    catch (Exception ex)
    {
        return "ERROR: " + ex.Message;
    }
}

static string LoadToken(string apiBase)
{
    try
    {
        var path = System.IO.Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "AnyClaw-Quicker", "token.txt");
        if (!File.Exists(path)) return null;
        foreach (var line in File.ReadAllLines(path))
        {
            var idx = line.IndexOf('=');
            if (idx > 0 && line.Substring(0, idx).Trim() == apiBase)
                return line.Substring(idx + 1).Trim();
        }
    }
    catch { }
    return null;
}

static void SaveToken(string apiBase, string token)
{
    try
    {
        var dir = System.IO.Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "AnyClaw-Quicker");
        Directory.CreateDirectory(dir);
        var path = System.IO.Path.Combine(dir, "token.txt");
        var lines = new List<string>();
        if (File.Exists(path))
        {
            foreach (var line in File.ReadAllLines(path))
            {
                var idx = line.IndexOf('=');
                if (idx > 0 && line.Substring(0, idx).Trim() != apiBase)
                    lines.Add(line);
            }
        }
        lines.Add(apiBase + "=" + token);
        File.WriteAllLines(path, lines);
    }
    catch { }
}

static void ClearToken(string apiBase)
{
    try
    {
        var path = System.IO.Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "AnyClaw-Quicker", "token.txt");
        if (!File.Exists(path)) return;
        var lines = File.ReadAllLines(path).Where(l => { var i = l.IndexOf('='); return i <= 0 || l.Substring(0, i).Trim() != apiBase; }).ToList();
        File.WriteAllLines(path, lines);
    }
    catch { }
}

static string JsonEscape(string s)
{
    if (s == null) return "";
    return s.Replace("\\", "\\\\").Replace("\"", "\\\"").Replace("\n", "\\n").Replace("\r", "\\r");
}

static DropShadowEffect CardShadow() => new DropShadowEffect { Color = Colors.Black, Opacity = 0.08, BlurRadius = 12, ShadowDepth = 2, Direction = 270 };
static DropShadowEffect SoftShadow() => new DropShadowEffect { Color = Colors.Black, Opacity = 0.06, BlurRadius = 8, ShadowDepth = 1, Direction = 270 };
static LinearGradientBrush GradBg() => new LinearGradientBrush(Color.FromRgb(248, 250, 252), Color.FromRgb(241, 245, 249), new Point(0, 0), new Point(1, 1));
static LinearGradientBrush GoldGrad() { var b = new LinearGradientBrush(Color.FromRgb(253, 230, 138), Color.FromRgb(251, 191, 36), new Point(0, 0), new Point(1, 1)); return b; }
static LinearGradientBrush AccentGrad() { var b = new LinearGradientBrush(Color.FromRgb(129, 140, 248), Color.FromRgb(99, 102, 241), new Point(0, 0), new Point(1, 1)); return b; }

static HttpClient CreateHttpClient()
{
    var handler = new HttpClientHandler();
    handler.ServerCertificateCustomValidationCallback = (sender, cert, chain, err) => true;
    var client = new HttpClient(handler);
    client.Timeout = TimeSpan.FromSeconds(30);
    client.DefaultRequestHeaders.Add("User-Agent", "AnyClaw-Quicker/1.0");
    return client;
}

class LoginWindow : Window
{
    public string Token { get; private set; }
    readonly string _apiBase;
    TextBox _emailBox;
    PasswordBox _pwdBox;
    Button _loginBtn;
    TextBlock _errorText;

    public LoginWindow(string apiBase, string defaultEmail = "", string defaultPassword = "")
    {
        _apiBase = apiBase;
        Title = "AnyClaw 登录";
        Width = 360;
        Height = 280;
        MinHeight = 280;
        WindowStartupLocation = WindowStartupLocation.CenterScreen;
        ResizeMode = ResizeMode.NoResize;
        Background = GradBg();

        var grid = new Grid();
        grid.RowDefinitions.Add(new RowDefinition { Height = new GridLength(1, GridUnitType.Star) });
        grid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });

        var sp = new StackPanel { Margin = new Thickness(28, 24, 28, 16) };
        sp.Children.Add(new TextBlock { Text = "邮箱", FontSize = 13, Margin = new Thickness(0, 0, 0, 6), Foreground = new SolidColorBrush(Color.FromRgb(71, 85, 105)) });
        _emailBox = new TextBox { Height = 36, Padding = new Thickness(12, 8, 12, 8), FontSize = 14 };
        _emailBox.Text = defaultEmail ?? "";
        _emailBox.BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240));
        _emailBox.BorderThickness = new Thickness(1, 1, 1, 1);
        sp.Children.Add(_emailBox);

        sp.Children.Add(new TextBlock { Text = "密码", FontSize = 13, Margin = new Thickness(0, 16, 0, 6), Foreground = new SolidColorBrush(Color.FromRgb(71, 85, 105)) });
        _pwdBox = new PasswordBox { Height = 36, Padding = new Thickness(12, 8, 12, 8), FontSize = 14 };
        _pwdBox.Password = defaultPassword ?? "";
        _pwdBox.BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240));
        _pwdBox.BorderThickness = new Thickness(1, 1, 1, 1);
        sp.Children.Add(_pwdBox);

        _errorText = new TextBlock { Foreground = new SolidColorBrush(Color.FromRgb(239, 68, 68)), FontSize = 12, Margin = new Thickness(0, 10, 0, 0), TextWrapping = TextWrapping.Wrap };
        sp.Children.Add(_errorText);

        Grid.SetRow(sp, 0);
        grid.Children.Add(sp);

        var btnRow = new StackPanel { Orientation = Orientation.Horizontal, Margin = new Thickness(28, 0, 28, 24) };
        _loginBtn = new Button { Content = "确定", Width = 88, Height = 38, Foreground = Brushes.White, BorderThickness = new Thickness(0, 0, 0, 0) };
        _loginBtn.Background = AccentGrad();
        _loginBtn.Effect = SoftShadow();
        _loginBtn.Click += OnLogin;
        var cancelBtn = new Button { Content = "取消", Width = 88, Height = 38, Margin = new Thickness(14, 0, 0, 0), Background = Brushes.White, BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240)), BorderThickness = new Thickness(1, 1, 1, 1) };
        cancelBtn.Effect = SoftShadow();
        cancelBtn.Click += (s, e) => { DialogResult = false; Close(); };
        btnRow.Children.Add(_loginBtn);
        btnRow.Children.Add(cancelBtn);
        Grid.SetRow(btnRow, 1);
        grid.Children.Add(btnRow);

        Content = grid;
    }

    void OnLogin(object s, RoutedEventArgs e)
    {
        var email = _emailBox.Text?.Trim();
        var password = _pwdBox.Password;
        if (string.IsNullOrEmpty(email)) { _errorText.Text = "请输入邮箱"; return; }
        if (string.IsNullOrEmpty(password)) { _errorText.Text = "请输入密码"; return; }

        _errorText.Text = "";
        _loginBtn.IsEnabled = false;
        var api = _apiBase;
        Task.Run(() =>
        {
            try
            {
                var t = DoLogin(api, email, password);
                Dispatcher.Invoke(() =>
                {
                    Token = t;
                    DialogResult = true;
                    Close();
                });
            }
            catch (Exception ex)
            {
                var msg = ex.Message;
                if (ex.InnerException != null) msg += " (" + ex.InnerException.Message + ")";
                Dispatcher.Invoke(() =>
                {
                    _errorText.Text = msg;
                    _loginBtn.IsEnabled = true;
                });
            }
        });
    }
}

static string DoLogin(string apiBase, string email, string password)
{
    using var client = CreateHttpClient();
    var body = "{\"email\":\"" + JsonEscape(email) + "\",\"password\":\"" + JsonEscape(password) + "\"}";
    var resp = client.PostAsync(apiBase.TrimEnd('/') + "/auth/login", new StringContent(body, Encoding.UTF8, "application/json")).GetAwaiter().GetResult();
    var text = resp.Content.ReadAsStringAsync().GetAwaiter().GetResult();
    if (!resp.IsSuccessStatusCode)
    {
        var m = Regex.Match(text, @"""error""\s*:\s*""([^""]*)""");
        throw new Exception(m.Success ? m.Groups[1].Value : text);
    }
    var tokMatch = Regex.Match(text, @"""access_token""\s*:\s*""([^""]+)""");
    if (tokMatch.Success) return tokMatch.Groups[1].Value;
    throw new Exception("登录响应无 access_token");
}

static UserInfo GetMe(string apiBase, string token)
{
    using var client = CreateHttpClient();
    client.DefaultRequestHeaders.Add("Authorization", "Bearer " + token);
    var resp = client.GetAsync(apiBase.TrimEnd('/') + "/me").GetAwaiter().GetResult();
    var text = resp.Content.ReadAsStringAsync().GetAwaiter().GetResult();
    if (!resp.IsSuccessStatusCode) throw new Exception("获取用户信息失败");
    var energyMatch = Regex.Match(text, @"""energy""\s*:\s*(\d+)");
    var emailMatch = Regex.Match(text, @"""email""\s*:\s*""([^""]*)""");
    return new UserInfo { Energy = energyMatch.Success ? int.Parse(energyMatch.Groups[1].Value) : 0, Email = emailMatch.Success ? emailMatch.Groups[1].Value : "" };
}

static List<InstanceItem> GetInstances(string apiBase, string token)
{
    using var client = CreateHttpClient();
    client.DefaultRequestHeaders.Add("Authorization", "Bearer " + token);
    var resp = client.GetAsync(apiBase.TrimEnd('/') + "/instances").GetAwaiter().GetResult();
    var text = resp.Content.ReadAsStringAsync().GetAwaiter().GetResult();
    if (!resp.IsSuccessStatusCode) throw new Exception("获取实例列表失败");
    var list = new List<InstanceItem>();
    var matches = Regex.Matches(text, @"""id""\s*:\s*(\d+)[^}]*?""name""\s*:\s*""([^""]*)""[^}]*?""status""\s*:\s*""([^""]*)""");
    foreach (Match m in matches)
    {
        if (int.TryParse(m.Groups[1].Value, out var id))
            list.Add(new InstanceItem { Id = id, Name = m.Groups[2].Value ?? "", Status = m.Groups[3].Value ?? "" });
    }
    return list;
}

static InstanceItem CreateInstance(string apiBase, string token, string name)
{
    using var client = CreateHttpClient();
    client.DefaultRequestHeaders.Add("Authorization", "Bearer " + token);
    var body = "{\"name\":\"" + JsonEscape(name ?? "小爪") + "\"}";
    var resp = client.PostAsync(apiBase.TrimEnd('/') + "/instances", new StringContent(body, Encoding.UTF8, "application/json")).GetAwaiter().GetResult();
    var text = resp.Content.ReadAsStringAsync().GetAwaiter().GetResult();
    if (!resp.IsSuccessStatusCode)
    {
        var m = Regex.Match(text, @"""error""\s*:\s*""([^""]*)""");
        throw new Exception(m.Success ? m.Groups[1].Value : text);
    }
    var idMatch = Regex.Match(text, @"""id""\s*:\s*(\d+)");
    var nameMatch = Regex.Match(text, @"""name""\s*:\s*""([^""]*)""");
    return new InstanceItem { Id = idMatch.Success ? int.Parse(idMatch.Groups[1].Value) : 0, Name = nameMatch.Success ? nameMatch.Groups[1].Value : "", Status = "creating" };
}

static void DeleteInstance(string apiBase, string token, int instanceId)
{
    using var client = CreateHttpClient();
    client.DefaultRequestHeaders.Add("Authorization", "Bearer " + token);
    var resp = client.DeleteAsync(apiBase.TrimEnd('/') + "/instances/" + instanceId).GetAwaiter().GetResult();
    if (!resp.IsSuccessStatusCode) throw new Exception("弃养失败");
}

static List<(string Id, string Content, bool IsUser)> GetMessages(string httpBase, string token, int instanceId, int limit = 10, string before = null)
{
    try
    {
        using var client = CreateHttpClient();
        client.DefaultRequestHeaders.Add("Authorization", "Bearer " + token);
        var url = httpBase.TrimEnd('/') + "/instances/" + instanceId + "/messages?limit=" + limit;
        if (!string.IsNullOrEmpty(before)) url += "&before=" + before;
        var resp = client.GetAsync(url).GetAwaiter().GetResult();
        var text = resp.Content.ReadAsStringAsync().GetAwaiter().GetResult();
        if (!resp.IsSuccessStatusCode) return new List<(string, string, bool)>();
        var list = new List<(string Id, string Content, bool IsUser)>();
        // 提取 messages 数组内容，逐条解析（避免长 content 导致单次正则失败）
        var arrMatch = Regex.Match(text, "\"messages\"\\s*:\\s*\\[([\\s\\S]*)\\]");
        if (!arrMatch.Success) return list;
        var arrBody = arrMatch.Groups[1].Value;
        // 按 },{ 分割每个消息对象（保留完整结构）
        var parts = Regex.Split(arrBody, "\\}\\s*,\\s*\\{");
        for (var i = 0; i < parts.Length; i++)
        {
            var raw = parts[i];
            if (i > 0) raw = "{" + raw;
            if (i < parts.Length - 1) raw = raw + "}";
            var idM = Regex.Match(raw, "\"id\"\\s*:\\s*(\\d+)");
            var roleM = Regex.Match(raw, "\"role\"\\s*:\\s*\"([^\"]*)\"");
            var contentM = Regex.Match(raw, "\"content\"\\s*:\\s*\"((?:[^\"\\\\]|\\\\\\\\.)*)\"");
            if (!idM.Success || !contentM.Success) continue;
            var content = UnescapeJsonString(contentM.Groups[1].Value ?? "");
            if (string.IsNullOrEmpty(content) || content.StartsWith("Thinking")) continue;
            var role = roleM.Success ? (roleM.Groups[1].Value ?? "") : "assistant";
            list.Add((idM.Groups[1].Value, content, role == "user"));
        }
        list.Sort((a, b) =>
        {
            if (long.TryParse(a.Id, out var ia) && long.TryParse(b.Id, out var ib))
                return ia.CompareTo(ib);
            return string.Compare(a.Id, b.Id, StringComparison.Ordinal);
        });
        return list;
    }
    catch { return new List<(string, string, bool)>(); }
}

static string UnescapeJsonString(string s)
{
    if (string.IsNullOrEmpty(s)) return s;
    return s.Replace("\\\"", "\"").Replace("\\\\", "\\").Replace("\\n", "\n").Replace("\\r", "\r");
}

// 解析 WebSocket 消息，优先用 JSON 解析（支持复杂 content 如错误信息），失败时回退到正则
static (string type, string content, string role, string messageId) ParseWsMessage(string json)
{
    if (string.IsNullOrEmpty(json)) return ("", "", "", "");
    try
    {
        using var doc = JsonDocument.Parse(json);
        var root = doc.RootElement;
        var type = root.TryGetProperty("type", out var t) ? t.GetString() ?? "" : "";
        var messageId = "";
        var content = "";
        var role = "";
        if (root.TryGetProperty("payload", out var payload))
        {
            if (payload.TryGetProperty("message_id", out var mid))
                messageId = mid.ValueKind == JsonValueKind.Number ? mid.GetInt64().ToString() : mid.GetString() ?? "";
            if (payload.TryGetProperty("content", out var c))
                content = c.GetString() ?? "";
            if (payload.TryGetProperty("role", out var r))
                role = r.GetString() ?? "";
        }
        return (type ?? "", content ?? "", role ?? "", messageId ?? "");
    }
    catch
    {
        var typeMatch = Regex.Match(json, "\"type\"\\s*:\\s*\"([^\"]+)\"");
        var msgIdMatch = Regex.Match(json, "\"message_id\"\\s*:\\s*(\\d+)");
        var contentMatch = Regex.Match(json, "\"content\"\\s*:\\s*\"((?:[^\"\\\\]|\\\\.)*)\"");
        var roleMatch = Regex.Match(json, "\"role\"\\s*:\\s*\"([^\"]+)\"");
        return (
            typeMatch.Success ? typeMatch.Groups[1].Value : "",
            contentMatch.Success ? UnescapeJsonString(contentMatch.Groups[1].Value) : "",
            roleMatch.Success ? roleMatch.Groups[1].Value : "",
            msgIdMatch.Success ? msgIdMatch.Groups[1].Value : ""
        );
    }
}

static string GetDesktopConfigPath(string apiBase)
{
    var dir = System.IO.Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "AnyClaw-Quicker");
    Directory.CreateDirectory(dir);
    var hash = Convert.ToBase64String(Encoding.UTF8.GetBytes(apiBase)).Replace("+", "_").Replace("/", "-").Replace("=", "");
    return System.IO.Path.Combine(dir, "desktop_" + hash + ".txt");
}

static List<(int Id, string SchemeId, double X, double Y)> LoadDesktopConfig(string apiBase)
{
    var list = new List<(int, string, double, double)>();
    try
    {
        var path = GetDesktopConfigPath(apiBase);
        if (!File.Exists(path)) return list;
        foreach (var line in File.ReadAllLines(path))
        {
            var parts = (line ?? "").Split('|');
            if (parts.Length >= 3 && int.TryParse(parts[0], out var id) && parts[1] == "1")
            {
                var scheme = parts[2].Trim();
                var schemeId = ColorSchemeById(scheme) != null ? scheme : "lobster";
                double x = parts.Length >= 4 && double.TryParse(parts[3], out var px) ? px : double.NaN;
                double y = parts.Length >= 5 && double.TryParse(parts[4], out var py) ? py : double.NaN;
                list.Add((id, schemeId, x, y));
            }
        }
    }
    catch { }
    return list;
}

static void SaveDesktopConfig(string apiBase, List<(int Id, bool Show, string SchemeId, double X, double Y)> items)
{
    try
    {
        var lines = items.Select(x => x.Id + "|" + (x.Show ? "1" : "0") + "|" + (x.SchemeId ?? "lobster") + "|" + x.X.ToString("F0") + "|" + x.Y.ToString("F0")).ToList();
        File.WriteAllLines(GetDesktopConfigPath(apiBase), lines);
    }
    catch { }
}

static void UpdateMascotPosition(string apiBase, int instanceId, double x, double y)
{
    try
    {
        var path = GetDesktopConfigPath(apiBase);
        if (!File.Exists(path)) return;
        var lines = File.ReadAllLines(path).ToList();
        for (var i = 0; i < lines.Count; i++)
        {
            var parts = (lines[i] ?? "").Split('|');
            if (parts.Length >= 3 && int.TryParse(parts[0], out var id) && id == instanceId && parts[1] == "1")
            {
                var scheme = parts[2].Trim();
                lines[i] = id + "|1|" + scheme + "|" + x.ToString("F0") + "|" + y.ToString("F0");
                break;
            }
        }
        File.WriteAllLines(path, lines);
    }
    catch { }
}

struct ColorScheme { public string Id; public string Name; public byte R1, G1, B1, R2, G2, B2; }
static readonly ColorScheme[] ColorSchemes = {
    new ColorScheme { Id = "lobster", Name = "龙虾红", R1 = 255, G1 = 77, B1 = 77, R2 = 153, G2 = 27, B2 = 27 },
    new ColorScheme { Id = "ocean", Name = "海洋蓝", R1 = 59, G1 = 130, B1 = 246, R2 = 30, G2 = 64, B2 = 175 },
    new ColorScheme { Id = "jade", Name = "翡翠绿", R1 = 16, G1 = 185, B1 = 129, R2 = 4, G2 = 120, B2 = 87 },
    new ColorScheme { Id = "coral", Name = "珊瑚橙", R1 = 249, G1 = 115, B1 = 22, R2 = 194, G2 = 65, B2 = 12 },
    new ColorScheme { Id = "lavender", Name = "薰衣草紫", R1 = 139, G1 = 92, B1 = 246, R2 = 91, G2 = 33, B2 = 182 },
    new ColorScheme { Id = "amber", Name = "琥珀金", R1 = 245, G1 = 158, B1 = 11, R2 = 217, G2 = 119, B2 = 6 },
    new ColorScheme { Id = "mint", Name = "薄荷青", R1 = 20, G1 = 184, B1 = 166, R2 = 13, G2 = 148, B2 = 136 },
    new ColorScheme { Id = "sakura", Name = "樱花粉", R1 = 236, G1 = 72, B1 = 153, R2 = 190, G2 = 24, B2 = 93 },
    new ColorScheme { Id = "indigo", Name = "靛蓝", R1 = 99, G1 = 102, B1 = 241, R2 = 67, G2 = 56, B2 = 202 },
    new ColorScheme { Id = "forest", Name = "森林绿", R1 = 34, G1 = 197, B1 = 94, R2 = 21, G2 = 128, B2 = 61 },
    new ColorScheme { Id = "rose", Name = "玫瑰", R1 = 244, G1 = 63, B1 = 94, R2 = 159, G2 = 18, B2 = 57 },
    new ColorScheme { Id = "sky", Name = "青空", R1 = 14, G1 = 165, B1 = 233, R2 = 12, G2 = 74, B2 = 110 }
};
static ColorScheme? ColorSchemeById(string id) { foreach (var s in ColorSchemes) if (s.Id == id) return s; return null; }

class UserInfo { public int Energy; public string Email; }
class InstanceItem { public int Id; public string Name; public string Status; }

static readonly List<ClawMascotWindow> s_mascots = new List<ClawMascotWindow>();

class ClawAdminWindow : Window
{
    readonly string _apiBase;
    readonly string _token;
    Point _dragStartScreen;

    public ClawAdminWindow(string apiBase, string token)
    {
        _apiBase = apiBase;
        _token = token;
        Title = "OpenClaw 管理";
        Width = 56;
        Height = 56;
        WindowStyle = WindowStyle.None;
        AllowsTransparency = true;
        Background = Brushes.Transparent;
        ResizeMode = ResizeMode.NoResize;
        WindowStartupLocation = WindowStartupLocation.Manual;
        ShowInTaskbar = false;
        Topmost = true;
        Left = SystemParameters.PrimaryScreenWidth - 80;
        Top = SystemParameters.PrimaryScreenHeight - 120;

        var canvas = new Canvas { Width = 56, Height = 56 };
        var tb = new TextBlock { Text = "⚙️", FontSize = 36, VerticalAlignment = VerticalAlignment.Center, HorizontalAlignment = HorizontalAlignment.Center };
        var vb = new Viewbox { Child = tb, Stretch = Stretch.Uniform };
        Canvas.SetLeft(vb, 8);
        Canvas.SetTop(vb, 8);
        vb.Width = 40;
        vb.Height = 40;
        canvas.Children.Add(vb);

        var exitMenu = new ContextMenu();
        var exitItem = new MenuItem { Header = "退出" };
        exitItem.Click += (s2, e2) => Close();
        exitMenu.Items.Add(exitItem);
        canvas.ContextMenu = exitMenu;

        canvas.MouseLeftButtonDown += (s, e) => { _dragStartScreen = PointToScreen(e.GetPosition(this)); canvas.CaptureMouse(); };
        canvas.MouseLeftButtonUp += (s, e) =>
        {
            canvas.ReleaseMouseCapture();
            var p = PointToScreen(e.GetPosition(this));
            if (Math.Abs(p.X - _dragStartScreen.X) < 4 && Math.Abs(p.Y - _dragStartScreen.Y) < 4)
            {
                var mgr = new LobsterManagementWindow(_apiBase, _token);
                mgr.Owner = this;
                mgr.ShowDialog();
            }
        };
        canvas.MouseMove += (s, e) =>
        {
            if (e.LeftButton == MouseButtonState.Pressed)
            {
                var p = PointToScreen(e.GetPosition(this));
                Left += p.X - _dragStartScreen.X;
                Top += p.Y - _dragStartScreen.Y;
                _dragStartScreen = p;
            }
        };
        Content = canvas;
    }
}

class LobsterManagementWindow : Window
{
    readonly string _apiBase;
    readonly string _token;
    StackPanel _listPanel;
    readonly Dictionary<int, CheckBox> _showChecks = new Dictionary<int, CheckBox>();
    readonly Dictionary<int, ComboBox> _schemeCombos = new Dictionary<int, ComboBox>();

    public LobsterManagementWindow(string apiBase, string token)
    {
        _apiBase = apiBase;
        _token = token;
        Title = "龙虾显示管理";
        Width = 520;
        Height = 520;
        MinHeight = 420;
        WindowStartupLocation = WindowStartupLocation.CenterOwner;
        Background = GradBg();

        var grid = new Grid();
        grid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        grid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        grid.RowDefinitions.Add(new RowDefinition { Height = new GridLength(1, GridUnitType.Star) });
        grid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });

        var headerRow = new Grid { Margin = new Thickness(24, 24, 24, 0) };
        headerRow.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        headerRow.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        headerRow.Children.Add(new TextBlock { Text = "龙虾显示管理", FontSize = 20, FontWeight = FontWeights.Bold, Foreground = new SolidColorBrush(Color.FromRgb(15, 23, 42)) });
        var toMainBtn = new Button { Content = "进入宠舍", Width = 80, Height = 32, FontSize = 13 };
        toMainBtn.Background = Brushes.White;
        toMainBtn.Foreground = new SolidColorBrush(Color.FromRgb(71, 85, 105));
        toMainBtn.BorderThickness = new Thickness(0);
        toMainBtn.Effect = SoftShadow();
        toMainBtn.Click += (s, e) => { new AnyClawMainWindow(_apiBase, _token).ShowDialog(); Refresh(); };
        Grid.SetColumn(toMainBtn, 1);
        headerRow.Children.Add(toMainBtn);
        Grid.SetRow(headerRow, 0);
        grid.Children.Add(headerRow);

        var hint = new TextBlock { Text = "勾选要显示的宠物，选择配色方案，点击应用", FontSize = 12, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), Margin = new Thickness(24, 10, 24, 12) };
        Grid.SetRow(hint, 1);
        grid.Children.Add(hint);

        _listPanel = new StackPanel();
        var scroll = new ScrollViewer { Content = _listPanel, VerticalScrollBarVisibility = ScrollBarVisibility.Auto, Padding = new Thickness(24, 0, 24, 0) };
        Grid.SetRow(scroll, 2);
        grid.Children.Add(scroll);

        var applyBtn = new Button { Content = "应用到桌面", Width = 140, Height = 42, Margin = new Thickness(24, 16, 24, 24), FontSize = 14, Foreground = Brushes.White, BorderThickness = new Thickness(0) };
        applyBtn.Background = AccentGrad();
        applyBtn.Effect = SoftShadow();
        applyBtn.Click += OnApply;
        Grid.SetRow(applyBtn, 3);
        grid.Children.Add(applyBtn);

        Content = grid;
        Loaded += (s, e) => Refresh();
    }

    void Refresh()
    {
        try
        {
            var instances = GetInstances(_apiBase, _token);
            var saved = LoadDesktopConfig(_apiBase);
            var savedDict = saved.ToDictionary(x => x.Id, x => x.SchemeId);
            _listPanel.Children.Clear();
            _showChecks.Clear();
            _schemeCombos.Clear();
            foreach (var i in instances)
            {
                var row = new Grid();
                row.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
                row.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
                row.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
                var chk = new CheckBox { IsChecked = savedDict.ContainsKey(i.Id), VerticalAlignment = VerticalAlignment.Center, Margin = new Thickness(0, 0, 12, 0) };
                _showChecks[i.Id] = chk;
                var nameTb = new TextBlock { Text = i.Name + (i.Status == "running" ? " (在线)" : i.Status == "creating" ? " (创建中)" : ""), VerticalAlignment = VerticalAlignment.Center, FontSize = 14 };
                var combo = new ComboBox { Width = 100, VerticalAlignment = VerticalAlignment.Center };
                var savedSchemeId = savedDict.ContainsKey(i.Id) && ColorSchemeById(savedDict[i.Id]) != null ? savedDict[i.Id] : "lobster";
                foreach (var s in ColorSchemes)
                {
                    var item = new ComboBoxItem { Content = s.Name, Tag = s.Id };
                    combo.Items.Add(item);
                    if (s.Id == savedSchemeId) combo.SelectedItem = item;
                }
                if (combo.SelectedItem == null && combo.Items.Count > 0) combo.SelectedIndex = 0;
                _schemeCombos[i.Id] = combo;
                Grid.SetColumn(chk, 0);
                Grid.SetColumn(nameTb, 1);
                Grid.SetColumn(combo, 2);
                row.Children.Add(chk);
                row.Children.Add(nameTb);
                row.Children.Add(combo);
                var card = new Border { Child = row, Padding = new Thickness(14, 12, 14, 12), Margin = new Thickness(0, 0, 0, 10), Background = Brushes.White, CornerRadius = new CornerRadius(10), Effect = SoftShadow() };
                _listPanel.Children.Add(card);
            }
        }
        catch (Exception ex) { MessageBox.Show("加载失败: " + ex.Message); }
    }

    void OnApply(object s, RoutedEventArgs e)
    {
        try
        {
            var instances = GetInstances(_apiBase, _token);
            var saved = LoadDesktopConfig(_apiBase);
            var posDict = saved.ToDictionary(x => x.Id, x => (x.X, x.Y));
            var items = instances.Select(i =>
            {
                _showChecks.TryGetValue(i.Id, out var chk);
                _schemeCombos.TryGetValue(i.Id, out var combo);
                var show = chk?.IsChecked == true;
                var schemeId = (combo?.SelectedItem as ComboBoxItem)?.Tag as string ?? "lobster";
                if (ColorSchemeById(schemeId) == null) schemeId = "lobster";
                var (sx, sy) = posDict.ContainsKey(i.Id) ? posDict[i.Id] : (double.NaN, double.NaN);
                return (i.Id, Show: show, SchemeId: schemeId, X: sx, Y: sy);
            }).ToList();
            SaveDesktopConfig(_apiBase, items.Select(x => (x.Id, x.Show, x.SchemeId, x.X, x.Y)).ToList());
            lock (s_mascots)
            {
                foreach (var m in s_mascots.ToList()) m.Close();
                s_mascots.Clear();
                var idx = 0;
                foreach (var (id, show, schemeId, x, y) in items.Where(x => x.Show))
                {
                    var inst = instances.FirstOrDefault(x => x.Id == id);
                    if (inst != null && inst.Status == "running")
                    {
                        var mascot = new ClawMascotWindow(_apiBase, _token, id, inst.Name, schemeId, idx++, x, y);
                        mascot.Show();
                    }
                }
            }
            Close();
        }
        catch (Exception ex) { MessageBox.Show("应用失败: " + ex.Message); }
    }
}

class ClawMascotWindow : Window
{
    readonly string _apiBase;
    readonly string _token;
    readonly int _instanceId;
    readonly string _instanceName;
    Point _dragStartScreen;
    ClientWebSocket _ws;
    CancellationTokenSource _cts;
    bool _connected;
    string _lastMessageContent = "";
    bool _hasUnread = false;
    bool _isChatWindowOpen = false;
    readonly List<ChatMsg> _messages = new List<ChatMsg>();

    // UI elements
    Canvas _canvas;
    Border _notificationBadge;
    TextBlock _badgeText;
    Window _quickReplyWindow;
    TextBox _quickReplyBox;
    TextBlock _quickReplyPreview;

    public ClawMascotWindow(string apiBase, string token, int instanceId, string instanceName, string schemeId, int positionIndex = 0, double savedX = double.NaN, double savedY = double.NaN)
    {
        _apiBase = apiBase;
        _token = token;
        _instanceId = instanceId;
        _instanceName = instanceName ?? "宠物";
        Title = _instanceName;
        Width = 56;
        Height = 56;
        WindowStyle = WindowStyle.None;
        AllowsTransparency = true;
        Background = Brushes.Transparent;
        ResizeMode = ResizeMode.NoResize;
        WindowStartupLocation = WindowStartupLocation.Manual;
        ShowInTaskbar = false;
        Topmost = true;
        if (!double.IsNaN(savedX) && !double.IsNaN(savedY))
        {
            Left = savedX;
            Top = savedY;
        }
        else
        {
            Left = SystemParameters.PrimaryScreenWidth - 80 - (positionIndex % 3) * 60;
            Top = SystemParameters.PrimaryScreenHeight - 120 - (positionIndex / 3) * 65;
        }
        lock (s_mascots) { s_mascots.Add(this); }
        Closed += (s, ev) =>
        {
            lock (s_mascots) { s_mascots.Remove(this); }
            UpdateMascotPosition(_apiBase, _instanceId, Left, Top);
            _cts?.Cancel();
            _ws?.Dispose();
        };

        var scheme = ColorSchemeById(schemeId ?? "lobster") ?? ColorSchemes[0];
        var grad = new LinearGradientBrush(
            Color.FromRgb(scheme.R1, scheme.G1, scheme.B1),
            Color.FromRgb(scheme.R2, scheme.G2, scheme.B2),
            new Point(0, 0), new Point(1, 1));

        _canvas = new Canvas { Width = 56, Height = 56 };
        var lobster = BuildLobsterShape(grad);
        var vb = new Viewbox { Child = lobster, Stretch = Stretch.Uniform };
        Canvas.SetLeft(vb, 4);
        Canvas.SetTop(vb, 4);
        vb.Width = 48;
        vb.Height = 48;
        _canvas.Children.Add(vb);

        // 未读消息徽章
        _notificationBadge = new Border
        {
            Width = 20,
            Height = 20,
            CornerRadius = new CornerRadius(10),
            Background = new SolidColorBrush(Color.FromRgb(239, 68, 68)),
            Visibility = Visibility.Collapsed,
            BorderThickness = new Thickness(2),
            BorderBrush = Brushes.White
        };
        _badgeText = new TextBlock
        {
            Text = "1",
            Foreground = Brushes.White,
            FontSize = 11,
            FontWeight = FontWeights.Bold,
            HorizontalAlignment = HorizontalAlignment.Center,
            VerticalAlignment = VerticalAlignment.Center
        };
        _notificationBadge.Child = _badgeText;
        Canvas.SetLeft(_notificationBadge, 36);
        Canvas.SetTop(_notificationBadge, 0);
        _canvas.Children.Add(_notificationBadge);

        // 右键菜单
        var menu = new ContextMenu();
        var chatItem = new MenuItem { Header = "打开对话" };
        chatItem.Click += (s2, e2) => OpenFullChat();
        var exitItem = new MenuItem { Header = "退出" };
        exitItem.Click += (s2, e2) => Close();
        menu.Items.Add(chatItem);
        menu.Items.Add(new Separator());
        menu.Items.Add(exitItem);
        _canvas.ContextMenu = menu;

        // 鼠标事件
        _canvas.MouseLeftButtonDown += (s, e) => { _dragStartScreen = PointToScreen(e.GetPosition(this)); _canvas.CaptureMouse(); };
        _canvas.MouseLeftButtonUp += OnMouseLeftButtonUp;
        _canvas.MouseMove += (s, e) =>
        {
            if (e.LeftButton == MouseButtonState.Pressed)
            {
                var p = PointToScreen(e.GetPosition(this));
                Left += p.X - _dragStartScreen.X;
                Top += p.Y - _dragStartScreen.Y;
                _dragStartScreen = p;
            }
        };

        Content = _canvas;
        Loaded += (s, e) => _ = ConnectWebSocket();
    }

    void OnMouseLeftButtonUp(object sender, MouseButtonEventArgs e)
    {
        _canvas.ReleaseMouseCapture();
        var p = PointToScreen(e.GetPosition(this));
        if (Math.Abs(p.X - _dragStartScreen.X) < 4 && Math.Abs(p.Y - _dragStartScreen.Y) < 4)
        {
            // 单击直接显示快捷回复（不管是否有未读消息）
            ShowQuickReply();
        }
    }

    public void OpenFullChat()
    {
        if (_isChatWindowOpen) return;
        _isChatWindowOpen = true;
        _hasUnread = false;
        UpdateBadge();

        var chat = new AnyClawChatWindow(this, _apiBase.Replace("http://", "ws://").Replace("https://", "wss://"), _token, _instanceId, _instanceName);
        chat.Owner = this;
        chat.Closed += (s, e) =>
        {
            _isChatWindowOpen = false;
        };
        chat.ShowDialog();
    }

    public List<ChatMsg> GetMessagesSnapshot() => new List<ChatMsg>(_messages);

    void UpdateBadge()
    {
        _notificationBadge.Visibility = _hasUnread ? Visibility.Visible : Visibility.Collapsed;
    }

    void ShowQuickReply()
    {
        // 如果窗口已打开，先关闭
        if (_quickReplyWindow != null && _quickReplyWindow.IsVisible)
        {
            _quickReplyWindow.Close();
        }

        // 判断是否有消息预览
        bool hasMessage = !string.IsNullOrEmpty(_lastMessageContent);
        int windowHeight = hasMessage ? 200 : 90;

        _quickReplyWindow = new Window
        {
            Width = 320,
            Height = windowHeight,
            WindowStyle = WindowStyle.None,
            AllowsTransparency = true,
            Background = Brushes.Transparent,
            ShowInTaskbar = false,
            Topmost = true,
            ResizeMode = ResizeMode.NoResize,
            Owner = this
        };

        // 定位在宠物图标上方
        var screenX = Left + (Width - 320) / 2;
        var screenY = Top - windowHeight - 10;
        _quickReplyWindow.Left = screenX;
        _quickReplyWindow.Top = screenY < 0 ? Top + Height + 10 : screenY;

        var border = new Border
        {
            Background = Brushes.White,
            CornerRadius = new CornerRadius(12),
            Padding = new Thickness(14, 12, 14, 12),
            Effect = CardShadow()
        };

        var sp = new StackPanel();

        // 如果有消息，显示消息预览区域
        if (hasMessage)
        {
            var headerRow = new Grid();
            headerRow.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
            headerRow.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });

            var title = new TextBlock
            {
                Text = "💬 最新消息",
                FontSize = 12,
                Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)),
                FontWeight = FontWeights.Medium,
                Margin = new Thickness(0, 0, 0, 6)
            };
            headerRow.Children.Add(title);

            var closeIcon = new TextBlock
            {
                Text = "✕",
                FontSize = 14,
                Foreground = new SolidColorBrush(Color.FromRgb(156, 163, 175)),
                Cursor = Cursors.Hand
            };
            closeIcon.MouseLeftButtonDown += (s, e) => _quickReplyWindow?.Close();
            Grid.SetColumn(closeIcon, 1);
            headerRow.Children.Add(closeIcon);

            sp.Children.Add(headerRow);

            var scrollViewer = new ScrollViewer
            {
                MaxHeight = 85,
                VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
                Margin = new Thickness(0, 0, 0, 10)
            };

            // 使用 Markdown 渲染，显示完整内容
            var mdTextBlock = CreateMarkdownTextBlock(_lastMessageContent, false);
            mdTextBlock.FontSize = 13;
            mdTextBlock.MaxWidth = 290;
            scrollViewer.Content = mdTextBlock;
            sp.Children.Add(scrollViewer);
        }
        else
        {
            // 首次打开，显示欢迎提示
            var tipRow = new StackPanel { Orientation = Orientation.Horizontal, Margin = new Thickness(0, 0, 0, 8) };
            var icon = new TextBlock { Text = "👋", FontSize = 16, Margin = new Thickness(0, 0, 6, 0) };
            var tip = new TextBlock
            {
                Text = "打个招呼开始聊天",
                FontSize = 13,
                Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)),
                VerticalAlignment = VerticalAlignment.Center
            };
            tipRow.Children.Add(icon);
            tipRow.Children.Add(tip);
            sp.Children.Add(tipRow);
        }

        var inputRow = new StackPanel { Orientation = Orientation.Horizontal };
        _quickReplyBox = new TextBox
        {
            Width = hasMessage ? 210 : 230,
            Height = 36,
            Padding = new Thickness(10, 8, 10, 8),
            FontSize = 13,
            VerticalContentAlignment = VerticalAlignment.Center,
            BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240)),
            BorderThickness = new Thickness(1)
        };
        _quickReplyBox.KeyDown += (s, ev) =>
        {
            if (ev.Key == Key.Enter)
            {
                ev.Handled = true;
                SendQuickReply();
            }
        };

        var sendBtn = new Button
        {
            Content = "发送",
            Width = 60,
            Height = 36,
            Margin = new Thickness(8, 0, 0, 0),
            FontSize = 13,
            Foreground = Brushes.White,
            Background = AccentGrad(),
            BorderThickness = new Thickness(0)
        };
        sendBtn.Click += (s, e) => SendQuickReply();

        inputRow.Children.Add(_quickReplyBox);
        inputRow.Children.Add(sendBtn);

        sp.Children.Add(inputRow);
        border.Child = sp;
        _quickReplyWindow.Content = border;

        // 点击外部或失去焦点时关闭
        _quickReplyWindow.Deactivated += (s, e) => _quickReplyWindow.Close();

        _quickReplyBox.Text = "";
        _quickReplyWindow.Show();
        _quickReplyBox.Focus();
    }

    void SendQuickReply()
    {
        var text = _quickReplyBox.Text?.Trim();
        if (string.IsNullOrEmpty(text)) return;

        // 直接尝试发送，不检查 _connected 标志
        if (_ws?.State == WebSocketState.Open)
        {
            try
            {
                var msg = "{\"type\":\"message.send\",\"payload\":{\"content\":\"" + JsonEscape(text) + "\"}}";
                _ws.SendAsync(new ArraySegment<byte>(Encoding.UTF8.GetBytes(msg)), WebSocketMessageType.Text, true, _cts.Token);
            }
            catch { }
        }

        _quickReplyWindow?.Close();
        _hasUnread = false;
        UpdateBadge();
    }

    public void SendMessage(string text)
    {
        if (string.IsNullOrEmpty(text) || _ws?.State != WebSocketState.Open) return;
        try
        {
            var msg = "{\"type\":\"message.send\",\"payload\":{\"content\":\"" + JsonEscape(text) + "\"}}";
            _ws.SendAsync(new ArraySegment<byte>(Encoding.UTF8.GetBytes(msg)), WebSocketMessageType.Text, true, _cts.Token);
        }
        catch { }
    }

    async Task ConnectWebSocket()
    {
        while (true)
        {
            try
            {
                var wsBase = _apiBase.Replace("http://", "ws://").Replace("https://", "wss://");
                var uri = new Uri(wsBase + "/instances/" + _instanceId + "/ws?token=" + Uri.EscapeDataString(_token));
                _ws = new ClientWebSocket();
                _cts = new CancellationTokenSource();
                await _ws.ConnectAsync(uri, _cts.Token);
                _connected = true;
                await ReceiveLoop();
            }
            catch (OperationCanceledException) { break; }
            catch (Exception ex) 
            { 
                _connected = false;
                // 显示连接失败状态（调试用）
                Dispatcher.Invoke(() =>
                {
                    _notificationBadge.Background = new SolidColorBrush(Color.FromRgb(156, 163, 175));
                    _notificationBadge.Visibility = Visibility.Visible;
                    _badgeText.Text = "!";
                });
            }
            finally
            {
                _ws?.Dispose();
                _ws = null;
            }
            // 断线后5秒重连
            await Task.Delay(5000);
            // 恢复徽章状态
            Dispatcher.Invoke(() => UpdateBadge());
        }
    }

    async Task ReceiveLoop()
    {
        var buf = new byte[16384];
        int msgCount = 0;
        while (_ws != null && _ws.State == WebSocketState.Open && !_cts.Token.IsCancellationRequested)
        {
            try
            {
                // 调试：进入循环，显示绿色点
                if (msgCount == 0)
                {
                    Dispatcher.Invoke(() =>
                    {
                        _notificationBadge.Background = new SolidColorBrush(Color.FromRgb(34, 197, 94));
                        _notificationBadge.Visibility = Visibility.Visible;
                        _badgeText.Text = "✓";
                    });
                }

                var sb = new StringBuilder();
                WebSocketReceiveResult result = null;
                bool firstReceive = true;
                do
                {
                    result = await _ws.ReceiveAsync(new ArraySegment<byte>(buf), _cts.Token);
                    
                    if (firstReceive)
                    {
                        firstReceive = false;
                        msgCount++;
                        // 收到第一条数据，显示黄色点
                        if (msgCount <= 3)
                        {
                            Dispatcher.Invoke(() =>
                            {
                                _notificationBadge.Background = new SolidColorBrush(Color.FromRgb(250, 204, 21));
                                _notificationBadge.Visibility = Visibility.Visible;
                                _badgeText.Text = msgCount.ToString();
                            });
                        }
                    }

                    if (result.MessageType == WebSocketMessageType.Close)
                    {
                        await _ws.CloseAsync(WebSocketCloseStatus.NormalClosure, "", CancellationToken.None);
                        return;
                    }
                    
                    if (result.Count > 0)
                    {
                        sb.Append(Encoding.UTF8.GetString(buf, 0, result.Count));
                    }
                } while (!result.EndOfMessage);

                var json = sb.ToString();
                if (string.IsNullOrEmpty(json)) continue;

                // 调试：收到完整消息显示蓝色点
                Dispatcher.Invoke(() =>
                {
                    _notificationBadge.Background = new SolidColorBrush(Color.FromRgb(59, 130, 246));
                    _notificationBadge.Visibility = Visibility.Visible;
                    _badgeText.Text = "●";
                });

                var (type, content, role, msgIdStr) = ParseWsMessage(json);

                // 调试：显示收到的消息类型和角色
                Dispatcher.Invoke(() =>
                {
                    _notificationBadge.Background = new SolidColorBrush(Color.FromRgb(139, 92, 246));
                    _notificationBadge.Visibility = Visibility.Visible;
                    _badgeText.Text = type == "message.create" ? "C" : (type == "message.update" ? "U" : (type.Length > 0 ? type.Substring(0, 1) : "?"));
                });

                // 处理助手消息 (role 可以是 assistant, model, 或为空；含错误信息也显示)
                bool isAssistantMessage = role != "user" && !string.IsNullOrEmpty(content);
                if ((type == "message.create" || type == "message.update") && isAssistantMessage && !content.StartsWith("Thinking"))
                {
                    try
                    {
                        Dispatcher.Invoke(() =>
                        {
                            try
                            {
                                // 存储消息
                                if (type == "message.update" && !string.IsNullOrEmpty(msgIdStr))
                                {
                                    var idx = _messages.FindIndex(m => m.Id == msgIdStr);
                                    if (idx >= 0) _messages[idx].Content = content;
                                }
                                else
                                {
                                    _messages.Add(new ChatMsg { Id = msgIdStr, Content = content, IsUser = false });
                                }

                                _lastMessageContent = content;

                                // 聊天窗口打开时，不显示通知
                                if (_isChatWindowOpen) return;

                                _hasUnread = true;
                                UpdateBadge();
                                ShowNotificationBubble(content);
                            }
                            catch { }
                        });
                    }
                    catch { }
                }
            }
            catch (OperationCanceledException) { return; }
            catch { return; }
        }
    }

    void ShowNotificationBubble(string content)
    {
        var popup = new Popup
        {
            Width = 300,
            MaxHeight = 180,
            Placement = PlacementMode.Top,
            PlacementTarget = this,
            AllowsTransparency = true,
            StaysOpen = true
        };

        var border = new Border
        {
            Background = new SolidColorBrush(Color.FromRgb(30, 41, 59)),
            CornerRadius = new CornerRadius(12),
            Padding = new Thickness(14, 12, 14, 12),
            Margin = new Thickness(0, 0, 0, 8),
            Effect = SoftShadow()
        };

        var scroll = new ScrollViewer
        {
            MaxHeight = 180,
            VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
            HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled
        };

        // 显示完整内容，不截断
        var displayText = _instanceName + ": " + content;
        var tb = new TextBlock
        {
            Text = displayText,
            FontSize = 13,
            Foreground = Brushes.White,
            TextWrapping = TextWrapping.Wrap,
            LineHeight = 20,
            TextTrimming = TextTrimming.None
        };

        scroll.Content = tb;
        border.Child = scroll;
        popup.Child = border;
        popup.IsOpen = true;

        // 8秒后自动关闭
        var timer = new System.Windows.Threading.DispatcherTimer { Interval = TimeSpan.FromSeconds(8) };
        timer.Tick += (s, e) =>
        {
            popup.IsOpen = false;
            timer.Stop();
        };
        timer.Start();

        // 点击气泡打开快捷回复
        border.MouseLeftButtonDown += (s, e) =>
        {
            popup.IsOpen = false;
            ShowQuickReply();
        };
    }

    static Canvas BuildLobsterShape(Brush fill)
    {
        var c = new Canvas { Width = 120, Height = 120 };
        var body = new PathShape { Data = Geometry.Parse("M60 10 C30 10 15 35 15 55 C15 75 30 95 45 100 L45 110 L55 110 L55 100 C55 100 60 102 65 100 L65 110 L75 110 L75 100 C90 95 105 75 105 55 C105 35 90 10 60 10Z"), Fill = fill };
        var leftClaw = new PathShape { Data = Geometry.Parse("M20 45 C5 40 0 50 5 60 C10 70 20 65 25 55 C28 48 25 45 20 45Z"), Fill = fill };
        var rightClaw = new PathShape { Data = Geometry.Parse("M100 45 C115 40 120 50 115 60 C110 70 100 65 95 55 C92 48 95 45 100 45Z"), Fill = fill };
        var eyeL = new Ellipse { Width = 12, Height = 12, Fill = new SolidColorBrush(Color.FromRgb(5, 8, 16)) };
        var eyeR = new Ellipse { Width = 12, Height = 12, Fill = new SolidColorBrush(Color.FromRgb(5, 8, 16)) };
        Canvas.SetLeft(eyeL, 39);
        Canvas.SetTop(eyeL, 29);
        Canvas.SetLeft(eyeR, 69);
        Canvas.SetTop(eyeR, 29);
        c.Children.Add(body);
        c.Children.Add(leftClaw);
        c.Children.Add(rightClaw);
        c.Children.Add(eyeL);
        c.Children.Add(eyeR);
        return c;
    }

    // Markdown 渲染方法（供快捷回复窗口使用）
    TextBlock CreateMarkdownTextBlock(string markdown, bool isUser)
    {
        var tb = new TextBlock
        {
            TextWrapping = TextWrapping.Wrap,
            FontSize = 14,
            LineHeight = 22,
            Foreground = isUser ? Brushes.White : new SolidColorBrush(Color.FromRgb(30, 41, 59)),
            TextTrimming = TextTrimming.None
        };

        // 简单的 Markdown 处理
        string processed = markdown;

        // 处理粗体 **text**
        processed = Regex.Replace(processed, @"\*\*(.+?)\*\*", "【B】$1【/B】");
        // 处理斜体 *text*
        processed = Regex.Replace(processed, @"\*(.+?)\*", "【I】$1【/I】");
        // 处理代码 `code`
        processed = Regex.Replace(processed, @"`([^`]+)`", "【CODE】$1【/CODE】");
        // 处理换行
        processed = processed.Replace("\n", "【BR】");

        // 解析并应用格式
        var parts = Regex.Split(processed, @"(【B】|【/B】|【I】|【/I】|【CODE】|【/CODE】|【BR】)");
        bool isBold = false, isItalic = false, isCode = false;

        foreach (var part in parts)
        {
            switch (part)
            {
                case "【B】": isBold = true; break;
                case "【/B】": isBold = false; break;
                case "【I】": isItalic = true; break;
                case "【/I】": isItalic = false; break;
                case "【CODE】": isCode = true; break;
                case "【/CODE】": isCode = false; break;
                case "【BR】": tb.Inlines.Add(new LineBreak()); break;
                default:
                    if (!string.IsNullOrEmpty(part))
                    {
                        var run = new Run(part);
                        if (isBold) run.FontWeight = FontWeights.Bold;
                        if (isItalic) run.FontStyle = FontStyles.Italic;
                        if (isCode)
                        {
                            run.FontFamily = new FontFamily("Consolas, Courier New, monospace");
                            run.Background = isUser ? new SolidColorBrush(Color.FromRgb(67, 56, 202)) : new SolidColorBrush(Color.FromRgb(241, 245, 249));
                            run.Foreground = isUser ? Brushes.White : new SolidColorBrush(Color.FromRgb(79, 70, 229));
                        }
                        tb.Inlines.Add(run);
                    }
                    break;
            }
        }

        return tb;
    }
}

class AnyClawMainWindow : Window
{
    readonly string _apiBase;
    readonly string _token;
    StackPanel _instancePanel;
    ScrollViewer _instanceScroll;
    TextBlock _energyText;
    TextBox _newNameBox;
    Button _adoptBtn;

    public AnyClawMainWindow(string apiBase, string token)
    {
        _apiBase = apiBase;
        _token = token;
        Title = "OpenClaw 宠舍";
        Width = 440;
        Height = 560;
        WindowStartupLocation = WindowStartupLocation.CenterScreen;
        Background = GradBg();

        var main = new StackPanel { Margin = new Thickness(20, 20, 20, 20) };

        var header = new Grid();
        header.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        header.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        header.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        var title = new TextBlock { Text = "OpenClaw 宠舍", FontSize = 22, FontWeight = FontWeights.Bold, Foreground = new SolidColorBrush(Color.FromRgb(15, 23, 42)) };
        Grid.SetColumn(title, 0);
        var refreshBtn = new Button { Content = "刷新", Width = 60, Height = 34, Margin = new Thickness(10, 0, 0, 0), FontSize = 13 };
        refreshBtn.Background = Brushes.White;
        refreshBtn.Foreground = new SolidColorBrush(Color.FromRgb(71, 85, 105));
        refreshBtn.BorderThickness = new Thickness(0, 0, 0, 0);
        refreshBtn.Effect = SoftShadow();
        refreshBtn.Click += (s, e) => Refresh();
        Grid.SetColumn(refreshBtn, 1);
        var logoutBtn = new Button { Content = "退出", Width = 60, Height = 34, Margin = new Thickness(10, 0, 0, 0), FontSize = 13 };
        logoutBtn.Background = Brushes.White;
        logoutBtn.Foreground = new SolidColorBrush(Color.FromRgb(71, 85, 105));
        logoutBtn.BorderThickness = new Thickness(0, 0, 0, 0);
        logoutBtn.Effect = SoftShadow();
        logoutBtn.Click += (s, e) => { ClearToken(_apiBase); Close(); };
        Grid.SetColumn(logoutBtn, 2);
        header.Children.Add(title);
        header.Children.Add(refreshBtn);
        header.Children.Add(logoutBtn);
        main.Children.Add(header);

        var energyPanel = new Border { Padding = new Thickness(20, 14, 20, 14), Margin = new Thickness(0, 20, 0, 0), CornerRadius = new CornerRadius(16), Effect = CardShadow() };
        energyPanel.Background = GoldGrad();
        var energyRow = new StackPanel { Orientation = Orientation.Horizontal };
        var coinIcon = new Viewbox { Width = 24, Height = 24, Margin = new Thickness(0, 0, 10, 0) };
        var coinCanvas = new Canvas { Width = 24, Height = 24 };
        var coin = new Ellipse { Width = 20, Height = 20, Fill = new SolidColorBrush(Color.FromRgb(251, 191, 36)), Stroke = new SolidColorBrush(Color.FromRgb(217, 119, 6)), StrokeThickness = 1.5 };
        Canvas.SetLeft(coin, 2);
        Canvas.SetTop(coin, 2);
        coinCanvas.Children.Add(coin);
        coinIcon.Child = coinCanvas;
        energyRow.Children.Add(coinIcon);
        energyRow.Children.Add(new TextBlock { Text = "我的金币 ", VerticalAlignment = VerticalAlignment.Center, FontSize = 15, Foreground = new SolidColorBrush(Color.FromRgb(120, 53, 15)) });
        _energyText = new TextBlock { Text = "0", FontWeight = FontWeights.Bold, FontSize = 22, Foreground = new SolidColorBrush(Color.FromRgb(120, 53, 15)), VerticalAlignment = VerticalAlignment.Center };
        energyRow.Children.Add(_energyText);
        energyPanel.Child = energyRow;
        main.Children.Add(energyPanel);

        var adoptCard = new Border { Background = Brushes.White, Padding = new Thickness(20, 16, 20, 16), Margin = new Thickness(0, 16, 0, 0), CornerRadius = new CornerRadius(16), Effect = CardShadow() };
        var adoptPanel = new StackPanel();
        adoptPanel.Children.Add(new TextBlock { Text = "领养 OpenClaw", FontSize = 16, FontWeight = FontWeights.SemiBold, Foreground = new SolidColorBrush(Color.FromRgb(15, 23, 42)) });
        adoptPanel.Children.Add(new TextBlock { Text = "每只宠物都有唯一的灵魂，擅长复杂任务、拥有超长记忆", FontSize = 12, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), Margin = new Thickness(0, 6, 0, 0) });
        var adoptRow = new StackPanel { Orientation = Orientation.Horizontal, Margin = new Thickness(0, 12, 0, 0) };
        _newNameBox = new TextBox { Width = 170, Height = 40, Padding = new Thickness(14, 10, 14, 10), VerticalContentAlignment = VerticalAlignment.Center, FontSize = 14 };
        _newNameBox.BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240));
        _newNameBox.BorderThickness = new Thickness(1, 1, 1, 1);
        _adoptBtn = new Button { Content = "领养 · 100 金币", Width = 140, Height = 40, Margin = new Thickness(14, 0, 0, 0), FontSize = 14, Foreground = Brushes.White, BorderThickness = new Thickness(0, 0, 0, 0) };
        _adoptBtn.Background = AccentGrad();
        _adoptBtn.Effect = SoftShadow();
        _adoptBtn.Click += OnAdopt;
        adoptRow.Children.Add(_newNameBox);
        adoptRow.Children.Add(_adoptBtn);
        adoptPanel.Children.Add(adoptRow);
        adoptCard.Child = adoptPanel;
        main.Children.Add(adoptCard);

        main.Children.Add(new TextBlock { Text = "我的宠舍", FontSize = 16, FontWeight = FontWeights.SemiBold, Foreground = new SolidColorBrush(Color.FromRgb(15, 23, 42)), Margin = new Thickness(0, 24, 0, 12) });
        _instancePanel = new StackPanel();
        _instanceScroll = new ScrollViewer { Content = _instancePanel, Height = 200, VerticalScrollBarVisibility = ScrollBarVisibility.Auto, Padding = new Thickness(2) };
        main.Children.Add(_instanceScroll);

        Content = main;
        Loaded += (s, e) => Refresh();
    }

    void Refresh()
    {
        try
        {
            var user = GetMe(_apiBase, _token);
            _energyText.Text = user.Energy.ToString();
            var instances = GetInstances(_apiBase, _token);
            _instancePanel.Children.Clear();
            foreach (var i in instances)
            {
                var statusText = i.Status == "running" ? "在线" : i.Status == "creating" ? "创建中" : i.Status == "error" ? "异常" : i.Status;
                var statusColor = i.Status == "running" ? Color.FromRgb(34, 197, 94) : i.Status == "creating" ? Color.FromRgb(245, 158, 11) : Color.FromRgb(239, 68, 68);
                var cardBorder = new Border
                {
                    Background = Brushes.White,
                    Padding = new Thickness(16, 14, 16, 14),
                    CornerRadius = new CornerRadius(12),
                    Effect = SoftShadow(),
                    Cursor = Cursors.Hand
                };
                var cardGrid = new Grid();
                cardGrid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
                cardGrid.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
                var nameTb = new TextBlock { Text = i.Name, FontSize = 15, FontWeight = FontWeights.Medium, Foreground = new SolidColorBrush(Color.FromRgb(15, 23, 42)), VerticalAlignment = VerticalAlignment.Center };
                var statusTb = new TextBlock { Text = statusText, FontSize = 12, Foreground = new SolidColorBrush(statusColor), VerticalAlignment = VerticalAlignment.Center };
                Grid.SetColumn(statusTb, 1);
                cardGrid.Children.Add(nameTb);
                cardGrid.Children.Add(statusTb);
                cardBorder.Child = cardGrid;
                var card = new ContentControl { Content = cardBorder, Margin = new Thickness(0, 0, 0, 10), Cursor = Cursors.Hand };
                var inst = i;
                card.MouseDoubleClick += (s, e) =>
                {
                    if (inst.Status == "running")
                    {
                        var chat = new AnyClawChatWindow(_apiBase.Replace("http://", "ws://").Replace("https://", "wss://"), _token, inst.Id, inst.Name);
                        chat.Owner = this;
                        chat.ShowDialog();
                    }
                };
                var abandonMenu = new ContextMenu();
                var abandonItem = new MenuItem { Header = "弃养" };
                abandonItem.Click += (s2, e2) =>
                {
                    if (MessageBox.Show("确定弃养「" + inst.Name + "」？弃养后无法恢复。", "确认", MessageBoxButton.YesNo) == MessageBoxResult.Yes)
                    {
                        try { DeleteInstance(_apiBase, _token, inst.Id); Refresh(); } catch (Exception ex) { MessageBox.Show(ex.Message); }
                    }
                };
                abandonMenu.Items.Add(abandonItem);
                card.ContextMenu = abandonMenu;
                _instancePanel.Children.Add(card);
            }
            _adoptBtn.IsEnabled = user.Energy >= 100;
        }
        catch (Exception ex)
        {
            MessageBox.Show("刷新失败: " + ex.Message);
        }
    }

    void OnAdopt(object s, RoutedEventArgs e)
    {
        var name = _newNameBox.Text?.Trim() ?? "小爪";
        _adoptBtn.IsEnabled = false;
        try
        {
            CreateInstance(_apiBase, _token, name);
            MessageBox.Show("领养成功！宠物正在创建中，约 1–2 分钟，请稍后刷新。");
            Refresh();
        }
        catch (Exception ex)
        {
            MessageBox.Show(ex.Message);
        }
        finally
        {
            _adoptBtn.IsEnabled = true;
        }
    }
}

class ListBoxInstanceItem { public InstanceItem Item; public string Display; public override string ToString() => Display; }

class ChatMsg { public string Id; public string Content; public bool IsUser; }

class AnyClawChatWindow : Window
{
    readonly string _apiBase;
    readonly string _token;
    readonly int _instanceId;
    readonly string _instanceName;
    readonly ClawMascotWindow _mascot;
    ClientWebSocket _ws;
    CancellationTokenSource _cts;
    StackPanel _msgPanel;
    ScrollViewer _scrollViewer;
    TextBox _inputBox;
    TextBlock _typingText;
    TextBlock _statusText;
    bool _connected;
    readonly List<ChatMsg> _messages = new List<ChatMsg>();
    System.Windows.Threading.DispatcherTimer _syncTimer;
    string _oldestMessageId = null;
    bool _isLoadingMore = false;
    bool _hasMoreHistory = true;

    // 独立模式构造函数（无主窗口）
    public AnyClawChatWindow(string apiBase, string token, int instanceId, string instanceName = "")
        : this(null, apiBase, token, instanceId, instanceName) { }

    // 附属模式构造函数（从 mascot 打开）
    public AnyClawChatWindow(ClawMascotWindow mascot, string apiBase, string token, int instanceId, string instanceName = "")
    {
        _mascot = mascot;
        _apiBase = apiBase.TrimEnd('/');
        _token = token;
        _instanceId = instanceId;
        _instanceName = string.IsNullOrEmpty(instanceName) ? "宠物" : instanceName;
        Title = _instanceName + " - OpenClaw";
        Width = 460;
        Height = 620;
        WindowStartupLocation = WindowStartupLocation.CenterScreen;
        Background = GradBg();

        var main = new Grid();
        main.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        main.RowDefinitions.Add(new RowDefinition { Height = new GridLength(1, GridUnitType.Star) });
        main.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });

        var header = new Border { Background = Brushes.White, Padding = new Thickness(20, 14, 20, 14), Effect = SoftShadow() };
        var headerGrid = new Grid();
        headerGrid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        headerGrid.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        headerGrid.Children.Add(new TextBlock { Text = _instanceName, FontSize = 18, FontWeight = FontWeights.SemiBold, Foreground = new SolidColorBrush(Color.FromRgb(15, 23, 42)), VerticalAlignment = VerticalAlignment.Center });
        _statusText = new TextBlock { Text = "连接中...", FontSize = 13, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), VerticalAlignment = VerticalAlignment.Center };
        Grid.SetColumn(_statusText, 1);
        headerGrid.Children.Add(_statusText);
        header.Child = headerGrid;
        Grid.SetRow(header, 0);
        main.Children.Add(header);

        _msgPanel = new StackPanel { Margin = new Thickness(20, 16, 20, 16) };
        _typingText = new TextBlock { Text = "", Margin = new Thickness(0, 10, 0, 0), FontSize = 13, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), FontStyle = FontStyles.Italic, Visibility = Visibility.Collapsed };
        var msgStack = new StackPanel();
        msgStack.Children.Add(_msgPanel);
        msgStack.Children.Add(_typingText);
        _scrollViewer = new ScrollViewer { Content = msgStack, VerticalScrollBarVisibility = ScrollBarVisibility.Auto, Padding = new Thickness(0) };
        _scrollViewer.Loaded += (s, e) => _scrollViewer.ScrollToEnd();
        Grid.SetRow(_scrollViewer, 1);
        main.Children.Add(_scrollViewer);

        var inputBorder = new Border { Background = Brushes.White, Padding = new Thickness(20, 16, 20, 20), Effect = SoftShadow() };
        var inputRow = new StackPanel { Orientation = Orientation.Horizontal };
        _inputBox = new TextBox { Width = 310, Height = 40, Padding = new Thickness(16, 10, 16, 10), FontSize = 14, VerticalContentAlignment = VerticalAlignment.Center };
        _inputBox.BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240));
        _inputBox.BorderThickness = new Thickness(1, 1, 1, 1);
        _inputBox.KeyDown += (s, ev) => { if (ev.Key == Key.Enter) { ev.Handled = true; SendMessage(); } };
        var sendBtn = new Button { Content = "发送", Width = 80, Height = 40, Margin = new Thickness(14, 0, 0, 0), FontSize = 14, Foreground = Brushes.White, BorderThickness = new Thickness(0, 0, 0, 0) };
        sendBtn.Background = AccentGrad();
        sendBtn.Effect = SoftShadow();
        sendBtn.Click += (s, e) => SendMessage();
        inputRow.Children.Add(_inputBox);
        inputRow.Children.Add(sendBtn);
        inputBorder.Child = inputRow;
        Grid.SetRow(inputBorder, 2);
        main.Children.Add(inputBorder);

        Content = main;
        Loaded += (s, e) => FetchHistoryAndConnect();
        Closing += (s, e) =>
        {
            _syncTimer?.Stop();
            // 只有独立模式才取消 WebSocket
            if (_mascot == null) _cts?.Cancel();
        };
    }

    void FetchHistoryAndConnect()
    {
        // 始终从 API 加载完整历史（含用户提问和 AI 回复），mascot 缓存仅有 AI 回复无用户消息
        LoadMoreHistory(true);
        _scrollViewer.ScrollChanged += OnScrollChanged;
    }

    void OnScrollChanged(object sender, ScrollChangedEventArgs e)
    {
        // 当滚动到顶部且还有更多历史时，加载更多
        if (_scrollViewer.VerticalOffset == 0 && _hasMoreHistory && !_isLoadingMore && _messages.Count > 0)
        {
            // 找到最小的消息ID作为 before 参数
            int minId = int.MaxValue;
            foreach (var m in _messages)
            {
                if (int.TryParse(m.Id, out var id) && id < minId) minId = id;
            }
            if (minId != int.MaxValue)
                LoadMoreHistory(false, minId.ToString());
        }
    }

    void LoadMoreHistory(bool isFirstLoad, string before = null)
    {
        if (_isLoadingMore) return;
        _isLoadingMore = true;

        var httpBase = _apiBase.Replace("wss://", "https://").Replace("ws://", "http://");
        Task.Run(() =>
        {
            var history = GetMessages(httpBase, _token, _instanceId, 30, before);
            Dispatcher.Invoke(() =>
            {
                if (isFirstLoad)
                {
                    _msgPanel.Children.Clear();
                    _messages.Clear();
                }

                if (history.Count == 0)
                {
                    _hasMoreHistory = false;
                    if (isFirstLoad)
                    {
                        // 附属模式：API 为空时使用 mascot 已缓存的消息作为历史
                        if (_mascot != null)
                        {
                            var mascotMsgs = _mascot.GetMessagesSnapshot();
                            if (mascotMsgs.Count > 0)
                            {
                                foreach (var m in mascotMsgs)
                                {
                                    _messages.Add(new ChatMsg { Id = m.Id, Content = m.Content, IsUser = m.IsUser });
                                    AddBubble(m.Content, m.IsUser, false);
                                }
                                ScrollToBottom();
                            }
                            else
                                AddBubble("打个招呼吧～", false, true);
                        }
                        else
                            AddBubble("打个招呼吧～", false, true);
                    }
                }
                else
                {
                    // 记录滚动位置和高度
                    var offset = _scrollViewer.VerticalOffset;
                    var extent = _scrollViewer.ExtentHeight;

                    // 将新消息插入到顶部（历史消息在前）
                    for (var i = history.Count - 1; i >= 0; i--)
                    {
                        var (id, content, isUser) = history[i];
                        _messages.Insert(0, new ChatMsg { Id = id, Content = content, IsUser = isUser });
                        AddBubbleToTop(content, isUser, false);
                    }

                    if (!isFirstLoad)
                    {
                        // 保持滚动位置（减去新增内容的高度）
                        _scrollViewer.ScrollToVerticalOffset(_scrollViewer.ExtentHeight - extent + offset);
                    }

                    // 附属模式：用 mascot 的 WebSocket 缓存补充 API 可能漏存的 assistant 回复
                    if (isFirstLoad && _mascot != null)
                    {
                        var assistantContents = new HashSet<string>(StringComparer.Ordinal);
                        foreach (var m in _messages)
                            if (!m.IsUser) assistantContents.Add(m.Content ?? "");
                        var added = false;
                        foreach (var m in _mascot.GetMessagesSnapshot())
                        {
                            if (!m.IsUser && !string.IsNullOrEmpty(m.Content) && !assistantContents.Contains(m.Content))
                            {
                                assistantContents.Add(m.Content);
                                _messages.Add(new ChatMsg { Id = m.Id, Content = m.Content, IsUser = false });
                                AddBubble(m.Content, false, false, m.Content.StartsWith("Error processing message"));
                                added = true;
                            }
                        }
                        if (added) ScrollToBottom();
                    }
                }

                if (isFirstLoad)
                {
                    ScrollToBottom();
                    _statusText.Text = "在线";
                    _statusText.Foreground = new SolidColorBrush(Color.FromRgb(34, 197, 94));

                    // 如果有 mascot，启动定时同步
                    if (_mascot != null)
                        StartMascotSync();
                    else
                        _ = Connect();
                }

                _isLoadingMore = false;
            });
        });
    }

    void StartMascotSync()
    {
        _syncTimer = new System.Windows.Threading.DispatcherTimer { Interval = TimeSpan.FromMilliseconds(100) };
        var lastMascotCount = _mascot.GetMessagesSnapshot().Count;
        _syncTimer.Tick += (s, e) =>
        {
            var mascotSnapshot = _mascot.GetMessagesSnapshot();
            // 检查 mascot 是否有新消息
            if (mascotSnapshot.Count > lastMascotCount)
            {
                for (var i = lastMascotCount; i < mascotSnapshot.Count; i++)
                {
                    var m = mascotSnapshot[i];
                    _messages.Add(new ChatMsg { Id = m.Id, Content = m.Content, IsUser = m.IsUser });
                    AddBubble(m.Content, m.IsUser, false);
                }
                lastMascotCount = mascotSnapshot.Count;
                ScrollToBottom();
            }
            // 同步更新已有消息
            for (var i = 0; i < _messages.Count && i < mascotSnapshot.Count; i++)
            {
                if (_messages[i].Id == mascotSnapshot[i].Id && _messages[i].Content != mascotSnapshot[i].Content)
                {
                    _messages[i].Content = mascotSnapshot[i].Content;
                    UpdateBubbleAt(i, mascotSnapshot[i].Content);
                }
            }
        };
        _syncTimer.Start();
    }

    void UpdateBubbleAt(int index, string content)
    {
        // 找到第 index 个助手消息并更新
        var assistantCount = 0;
        for (var i = 0; i < _msgPanel.Children.Count; i++)
        {
            var wrap = _msgPanel.Children[i] as StackPanel;
            if (wrap?.Tag as string == "assistant")
            {
                if (assistantCount == index || (assistantCount == index - 1 && index > 0))
                {
                    if ((wrap.Children[0] as Border)?.Child is TextBlock tb)
                    {
                        tb.Text = content;
                        ScrollToBottom();
                    }
                    return;
                }
                assistantCount++;
            }
        }
    }

    async Task Connect()
    {
        var uri = new Uri(_apiBase + "/instances/" + _instanceId + "/ws?token=" + Uri.EscapeDataString(_token));
        _ws = new ClientWebSocket();
        _cts = new CancellationTokenSource();
        try
        {
            await _ws.ConnectAsync(uri, _cts.Token);
            _connected = true;
            Dispatcher.Invoke(() =>
            {
                _statusText.Text = "在线";
                _statusText.Foreground = new SolidColorBrush(Color.FromRgb(34, 197, 94));
                if (_messages.Count == 0) AddBubble("打个招呼吧～", false, true);
            });
            _ = ReceiveLoop();
        }
        catch (Exception ex)
        {
            Dispatcher.Invoke(() =>
            {
                _statusText.Text = "连接失败";
                _statusText.Foreground = Brushes.Red;
                AddBubble("连接失败: " + ex.Message, false, true);
            });
        }
    }

    void SendMessage()
    {
        var text = _inputBox.Text?.Trim();
        if (string.IsNullOrEmpty(text)) return;

        AddBubble(text, true, false);
        _inputBox.Clear();
        ScrollToBottom();

        // 如果有 mascot，通过 mascot 发送；否则自己发送
        if (_mascot != null)
        {
            _mascot.SendMessage(text);
            // 立即同步到本地消息列表
            _messages.Add(new ChatMsg { Id = "", Content = text, IsUser = true });
        }
        else if (_connected && _ws?.State == WebSocketState.Open)
        {
            var msg = "{\"type\":\"message.send\",\"payload\":{\"content\":\"" + JsonEscape(text) + "\"}}";
            _ws.SendAsync(new ArraySegment<byte>(Encoding.UTF8.GetBytes(msg)), WebSocketMessageType.Text, true, _cts.Token);
        }
    }

    async Task ReceiveLoop()
    {
        var buf = new byte[16384];
        try
        {
            while (_ws != null && _ws.State == WebSocketState.Open && !_cts.Token.IsCancellationRequested)
            {
                var sb = new StringBuilder();
                WebSocketReceiveResult result;
                do
                {
                    result = await _ws.ReceiveAsync(new ArraySegment<byte>(buf), _cts.Token);
                    sb.Append(Encoding.UTF8.GetString(buf, 0, result.Count));
                } while (!result.EndOfMessage);
                var json = sb.ToString();
                if (string.IsNullOrEmpty(json)) continue;
                var (type, content, _, msgId) = ParseWsMessage(json);
                Dispatcher.Invoke(() =>
                {
                    if (type == "typing.start")
                    {
                        _typingText.Text = "正在思考...";
                        _typingText.Visibility = Visibility.Visible;
                        ScrollToBottom();
                    }
                    else if (type == "typing.stop")
                    {
                        _typingText.Text = "";
                        _typingText.Visibility = Visibility.Collapsed;
                    }
                    else if ((type == "message.create" || type == "message.update") && !string.IsNullOrEmpty(content) && !content.StartsWith("Thinking"))
                    {
                        if (type == "message.update" && !string.IsNullOrEmpty(msgId))
                        {
                            var idx = _messages.FindIndex(m => m.Id == msgId);
                            if (idx >= 0)
                            {
                                _messages[idx].Content = content;
                                UpdateLastBubble(content);
                                return;
                            }
                        }
                        var m = new ChatMsg { Id = msgId, Content = content, IsUser = false };
                        _messages.Add(m);
                        AddBubble(content, false, false, content.StartsWith("Error processing message"));
                    }
                });
            }
        }
        catch (OperationCanceledException) { }
        catch (Exception ex)
        {
            Dispatcher.Invoke(() =>
            {
                _statusText.Text = "已断开";
                _statusText.Foreground = Brushes.Red;
                AddBubble("连接断开: " + ex.Message, false, true);
            });
        }
    }

    void AddBubble(string content, bool isUser, bool isSystem, bool isError = false)
    {
        var wrap = CreateBubbleWrap(content, isUser, isSystem, isError);
        _msgPanel.Children.Add(wrap);
        ScrollToBottom();
    }

    void AddBubbleToTop(string content, bool isUser, bool isSystem, bool isError = false)
    {
        var wrap = CreateBubbleWrap(content, isUser, isSystem, isError);
        if (_msgPanel.Children.Count > 0)
            _msgPanel.Children.Insert(0, wrap);
        else
            _msgPanel.Children.Add(wrap);
    }

    StackPanel CreateBubbleWrap(string content, bool isUser, bool isSystem, bool isError = false)
    {
        var wrap = new StackPanel { Orientation = Orientation.Horizontal, Margin = new Thickness(0, 8, 0, 0), Tag = isSystem ? "sys" : (isUser ? "user" : "assistant") };
        wrap.HorizontalAlignment = isUser ? HorizontalAlignment.Right : HorizontalAlignment.Left;

        var bubbleContainer = new Border
        {
            CornerRadius = new CornerRadius(16, 16, isUser ? 4 : 16, isUser ? 16 : 4),
            Padding = new Thickness(16, 12, 16, 12),
            MaxWidth = 320
        };

        if (isError)
        {
            bubbleContainer.Background = new SolidColorBrush(Color.FromRgb(254, 242, 242));
            bubbleContainer.BorderBrush = new SolidColorBrush(Color.FromRgb(254, 202, 202));
            bubbleContainer.BorderThickness = new Thickness(1, 1, 1, 1);
        }
        else if (isSystem)
        {
            bubbleContainer.Background = new SolidColorBrush(Color.FromRgb(241, 245, 249));
            bubbleContainer.BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240));
            bubbleContainer.BorderThickness = new Thickness(1, 1, 1, 1);
        }
        else if (isUser)
        {
            bubbleContainer.Background = AccentGrad();
            bubbleContainer.Effect = SoftShadow();
        }
        else
        {
            bubbleContainer.Background = Brushes.White;
            bubbleContainer.BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240));
            bubbleContainer.BorderThickness = new Thickness(1, 1, 1, 1);
            bubbleContainer.Effect = SoftShadow();
        }

        var contentStack = new StackPanel();

        // Markdown 渲染（完整内容，不截断）
        var mdTextBlock = CreateMarkdownTextBlock(content, isUser);
        contentStack.Children.Add(mdTextBlock);

        // 长消息添加展开按钮，点击在更大窗口查看
        bool isLongMessage = content.Length > 300 || content.Contains("```") || content.Contains("\n\n");
        if (isLongMessage && !isUser)
        {
            var expandBtn = new TextBlock
            {
                Text = "在窗口中查看",
                FontSize = 12,
                Foreground = new SolidColorBrush(Color.FromRgb(99, 102, 241)),
                FontWeight = FontWeights.Medium,
                Margin = new Thickness(0, 8, 0, 0),
                Cursor = Cursors.Hand
            };
            expandBtn.MouseLeftButtonDown += (s, e) => ShowExpandedMessage(content, _instanceName);
            contentStack.Children.Add(expandBtn);
        }

        bubbleContainer.Child = contentStack;
        wrap.Children.Add(bubbleContainer);
        return wrap;
    }

    TextBlock CreateMarkdownTextBlock(string markdown, bool isUser)
    {
        var tb = new TextBlock
        {
            TextWrapping = TextWrapping.Wrap,
            FontSize = 14,
            LineHeight = 22,
            Foreground = isUser ? Brushes.White : new SolidColorBrush(Color.FromRgb(30, 41, 59)),
            TextTrimming = TextTrimming.None
        };

        // 简单的 Markdown 处理
        string processed = markdown;

        // 处理粗体 **text**
        processed = Regex.Replace(processed, @"\*\*(.+?)\*\*", "【B】$1【/B】");
        // 处理斜体 *text*
        processed = Regex.Replace(processed, @"\*(.+?)\*", "【I】$1【/I】");
        // 处理代码 `code`
        processed = Regex.Replace(processed, @"`([^`]+)`", "【CODE】$1【/CODE】");
        // 处理换行
        processed = processed.Replace("\n", "【BR】");

        // 解析并应用格式
        var parts = Regex.Split(processed, @"(【B】|【/B】|【I】|【/I】|【CODE】|【/CODE】|【BR】)");
        bool isBold = false, isItalic = false, isCode = false;

        foreach (var part in parts)
        {
            switch (part)
            {
                case "【B】": isBold = true; break;
                case "【/B】": isBold = false; break;
                case "【I】": isItalic = true; break;
                case "【/I】": isItalic = false; break;
                case "【CODE】": isCode = true; break;
                case "【/CODE】": isCode = false; break;
                case "【BR】": tb.Inlines.Add(new LineBreak()); break;
                default:
                    if (!string.IsNullOrEmpty(part))
                    {
                        var run = new Run(part);
                        if (isBold) run.FontWeight = FontWeights.Bold;
                        if (isItalic) run.FontStyle = FontStyles.Italic;
                        if (isCode)
                        {
                            run.FontFamily = new FontFamily("Consolas, Courier New, monospace");
                            run.Background = isUser ? new SolidColorBrush(Color.FromRgb(67, 56, 202)) : new SolidColorBrush(Color.FromRgb(241, 245, 249));
                            run.Foreground = isUser ? Brushes.White : new SolidColorBrush(Color.FromRgb(79, 70, 229));
                        }
                        tb.Inlines.Add(run);
                    }
                    break;
            }
        }

        return tb;
    }

    void ShowExpandedMessage(string content, string title)
    {
        var expandWindow = new Window
        {
            Title = title + " - 完整消息",
            Width = 700,
            Height = 500,
            WindowStartupLocation = WindowStartupLocation.CenterScreen,
            Background = GradBg()
        };

        var mainPanel = new StackPanel { Margin = new Thickness(20) };

        // 标题栏
        var header = new Grid { Margin = new Thickness(0, 0, 0, 16) };
        header.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        header.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });

        var titleText = new TextBlock
        {
            Text = "完整消息",
            FontSize = 18,
            FontWeight = FontWeights.Bold,
            Foreground = new SolidColorBrush(Color.FromRgb(15, 23, 42))
        };
        header.Children.Add(titleText);

        var closeBtn = new Button
        {
            Content = "✕",
            Width = 32,
            Height = 32,
            FontSize = 14,
            Background = Brushes.Transparent,
            BorderThickness = new Thickness(0),
            Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139))
        };
        closeBtn.Click += (s, e) => expandWindow.Close();
        Grid.SetColumn(closeBtn, 1);
        header.Children.Add(closeBtn);

        mainPanel.Children.Add(header);

        // 消息内容区域
        var scrollViewer = new ScrollViewer
        {
            VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
            MaxHeight = 380
        };

        var contentBorder = new Border
        {
            Background = Brushes.White,
            CornerRadius = new CornerRadius(12),
            Padding = new Thickness(20),
            Effect = CardShadow()
        };

        var contentText = CreateMarkdownTextBlock(content, false);
        contentText.FontSize = 15;
        contentText.LineHeight = 26;

        contentBorder.Child = contentText;
        scrollViewer.Content = contentBorder;
        mainPanel.Children.Add(scrollViewer);

        expandWindow.Content = mainPanel;
        expandWindow.ShowDialog();
    }

    void UpdateLastBubble(string content)
    {
        for (var i = _msgPanel.Children.Count - 1; i >= 0; i--)
        {
            var wrap = _msgPanel.Children[i] as StackPanel;
            if (wrap?.Tag as string == "assistant" && wrap.Children.Count > 0)
            {
                var bubble = wrap.Children[0] as Border;
                if (bubble?.Child is StackPanel sp && sp.Children.Count > 0)
                {
                    // 更新第一个 TextBlock（内容区域）
                    if (sp.Children[0] is TextBlock oldTb)
                    {
                        var newTb = CreateMarkdownTextBlock(content, false);
                        sp.Children[0] = newTb;
                        ScrollToBottom();
                    }
                }
                return;
            }
        }
    }

    void ScrollToBottom()
    {
        _scrollViewer.ScrollToEnd();
    }
}
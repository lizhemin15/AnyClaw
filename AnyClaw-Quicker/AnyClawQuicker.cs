using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Net.Http;
using System.Net.WebSockets;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Controls.Primitives;
using System.Windows.Documents;
using System.Windows.Input;
using System.Windows.Media;
using System.Windows.Media.Imaging;
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
        Width = 400;
        Height = 460;
        MinHeight = 420;
        WindowStartupLocation = WindowStartupLocation.CenterScreen;
        ResizeMode = ResizeMode.NoResize;
        Topmost = true;
        Background = GradBg();

        var main = new Border { Margin = new Thickness(24, 24, 24, 24), Padding = new Thickness(32, 28, 32, 28), Background = Brushes.White, CornerRadius = new CornerRadius(20), Effect = CardShadow(), BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240)), BorderThickness = new Thickness(1, 1, 1, 1) };

        var stack = new StackPanel();
        stack.Children.Add(new TextBlock { Text = "登录", FontSize = 22, FontWeight = FontWeights.SemiBold, Foreground = new SolidColorBrush(Color.FromRgb(30, 41, 59)), Margin = new Thickness(0, 0, 0, 24) });

        stack.Children.Add(new TextBlock { Text = "邮箱", FontSize = 13, FontWeight = FontWeights.Medium, Margin = new Thickness(0, 0, 0, 8), Foreground = new SolidColorBrush(Color.FromRgb(71, 85, 105)) });
        _emailBox = new TextBox { Height = 44, Padding = new Thickness(14, 10, 14, 10), FontSize = 14 };
        _emailBox.Text = defaultEmail ?? "";
        _emailBox.BorderBrush = new SolidColorBrush(Color.FromRgb(203, 213, 225));
        _emailBox.BorderThickness = new Thickness(1, 1, 1, 1);
        _emailBox.Margin = new Thickness(0, 0, 0, 20);
        stack.Children.Add(_emailBox);

        stack.Children.Add(new TextBlock { Text = "密码", FontSize = 13, FontWeight = FontWeights.Medium, Margin = new Thickness(0, 0, 0, 8), Foreground = new SolidColorBrush(Color.FromRgb(71, 85, 105)) });
        _pwdBox = new PasswordBox { Height = 44, Padding = new Thickness(14, 10, 14, 10), FontSize = 14 };
        _pwdBox.Password = defaultPassword ?? "";
        _pwdBox.BorderBrush = new SolidColorBrush(Color.FromRgb(203, 213, 225));
        _pwdBox.BorderThickness = new Thickness(1, 1, 1, 1);
        _pwdBox.Margin = new Thickness(0, 0, 0, 16);
        stack.Children.Add(_pwdBox);

        _errorText = new TextBlock { Foreground = new SolidColorBrush(Color.FromRgb(239, 68, 68)), FontSize = 12, Margin = new Thickness(0, 0, 0, 16), TextWrapping = TextWrapping.Wrap, Visibility = Visibility.Collapsed };
        stack.Children.Add(_errorText);

        _loginBtn = new Button { Content = "登录", Height = 48, Foreground = Brushes.White, BorderThickness = new Thickness(0, 0, 0, 0) };
        _loginBtn.Background = new SolidColorBrush(Color.FromRgb(30, 41, 59));
        _loginBtn.Effect = SoftShadow();
        _loginBtn.FontSize = 15;
        _loginBtn.Click += OnLogin;
        stack.Children.Add(_loginBtn);

        var regRow = new StackPanel { Orientation = Orientation.Horizontal, Margin = new Thickness(0, 20, 0, 0), HorizontalAlignment = HorizontalAlignment.Center };
        regRow.Children.Add(new TextBlock { Text = "还没有账号？ ", FontSize = 13, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), VerticalAlignment = VerticalAlignment.Center });
        var regBtn = new Button { Content = "注册", Background = Brushes.Transparent, Foreground = new SolidColorBrush(Color.FromRgb(30, 41, 59)), BorderThickness = new Thickness(0, 0, 0, 0), FontWeight = FontWeights.Medium, FontSize = 13, Padding = new Thickness(4, 2, 4, 2), Cursor = Cursors.Hand };
        regBtn.Click += (s, e) =>
        {
            var regWin = new RegisterWindow(_apiBase);
            regWin.Owner = this;
            if (regWin.ShowDialog() == true && !string.IsNullOrEmpty(regWin.Token))
            {
                Token = regWin.Token;
                DialogResult = true;
                Close();
            }
        };
        regRow.Children.Add(regBtn);
        stack.Children.Add(regRow);

        main.Child = stack;
        Content = main;
    }

    void OnLogin(object s, RoutedEventArgs e)
    {
        var email = _emailBox.Text?.Trim();
        var password = _pwdBox.Password;
        if (string.IsNullOrEmpty(email)) { _errorText.Text = "请输入邮箱"; _errorText.Visibility = Visibility.Visible; return; }
        if (string.IsNullOrEmpty(password)) { _errorText.Text = "请输入密码"; _errorText.Visibility = Visibility.Visible; return; }

        _errorText.Text = "";
        _errorText.Visibility = Visibility.Collapsed;
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
                    _errorText.Visibility = Visibility.Visible;
                    _loginBtn.IsEnabled = true;
                });
            }
        });
    }
}

class RegisterWindow : Window
{
    public string Token { get; private set; }
    readonly string _apiBase;
    readonly bool _verificationRequired;
    TextBox _emailBox;
    TextBox _codeBox;
    StackPanel _codePanel;
    StackPanel _pwdPanel;
    PasswordBox _pwdBox;
    Button _primaryBtn;
    TextBlock _errorText;
    int _step = 1;

    public RegisterWindow(string apiBase)
    {
        _apiBase = apiBase;
        _verificationRequired = GetAuthConfigVerificationRequired(apiBase);
        Title = "AnyClaw 注册";
        Width = 400;
        Height = 420;
        MinHeight = 380;
        WindowStartupLocation = WindowStartupLocation.CenterOwner;
        ResizeMode = ResizeMode.NoResize;
        Topmost = true;
        Background = GradBg();

        var main = new Border { Margin = new Thickness(24, 24, 24, 24), Padding = new Thickness(32, 28, 32, 28), Background = Brushes.White, CornerRadius = new CornerRadius(20), Effect = CardShadow(), BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240)), BorderThickness = new Thickness(1, 1, 1, 1) };

        var stack = new StackPanel();
        stack.Children.Add(new TextBlock { Text = "注册", FontSize = 22, FontWeight = FontWeights.SemiBold, Foreground = new SolidColorBrush(Color.FromRgb(30, 41, 59)), Margin = new Thickness(0, 0, 0, 24) });

        stack.Children.Add(new TextBlock { Text = "邮箱", FontSize = 13, FontWeight = FontWeights.Medium, Margin = new Thickness(0, 0, 0, 8), Foreground = new SolidColorBrush(Color.FromRgb(71, 85, 105)) });
        _emailBox = new TextBox { Height = 44, Padding = new Thickness(14, 10, 14, 10), FontSize = 14 };
        _emailBox.BorderBrush = new SolidColorBrush(Color.FromRgb(203, 213, 225));
        _emailBox.BorderThickness = new Thickness(1, 1, 1, 1);
        _emailBox.Margin = new Thickness(0, 0, 0, 20);
        stack.Children.Add(_emailBox);

        if (_verificationRequired)
        {
            _codePanel = new StackPanel { Margin = new Thickness(0, 0, 0, 20), Visibility = Visibility.Collapsed };
            _codePanel.Children.Add(new TextBlock { Text = "验证码", FontSize = 13, FontWeight = FontWeights.Medium, Margin = new Thickness(0, 0, 0, 8), Foreground = new SolidColorBrush(Color.FromRgb(71, 85, 105)) });
            _codeBox = new TextBox { Height = 44, Padding = new Thickness(14, 10, 14, 10), FontSize = 14, MaxLength = 6 };
            _codeBox.BorderBrush = new SolidColorBrush(Color.FromRgb(203, 213, 225));
            _codeBox.BorderThickness = new Thickness(1, 1, 1, 1);
            _codeBox.PreviewTextInput += (s, e) => { e.Handled = !Regex.IsMatch(e.Text, @"^\d*$"); };
            _codePanel.Children.Add(_codeBox);
            stack.Children.Add(_codePanel);
        }

        _pwdPanel = new StackPanel { Margin = new Thickness(0, 0, 0, 16) };
        _pwdPanel.Children.Add(new TextBlock { Text = "密码（至少 6 位）", FontSize = 13, FontWeight = FontWeights.Medium, Margin = new Thickness(0, 0, 0, 8), Foreground = new SolidColorBrush(Color.FromRgb(71, 85, 105)) });
        _pwdBox = new PasswordBox { Height = 44, Padding = new Thickness(14, 10, 14, 10), FontSize = 14 };
        _pwdBox.BorderBrush = new SolidColorBrush(Color.FromRgb(203, 213, 225));
        _pwdBox.BorderThickness = new Thickness(1, 1, 1, 1);
        _pwdPanel.Children.Add(_pwdBox);
        stack.Children.Add(_pwdPanel);
        if (_verificationRequired) _pwdPanel.Visibility = Visibility.Collapsed;

        _errorText = new TextBlock { Foreground = new SolidColorBrush(Color.FromRgb(239, 68, 68)), FontSize = 12, Margin = new Thickness(0, 0, 0, 16), TextWrapping = TextWrapping.Wrap, Visibility = Visibility.Collapsed };
        stack.Children.Add(_errorText);

        _primaryBtn = new Button { Content = _verificationRequired ? "发送验证码" : "注册", Height = 48, Foreground = Brushes.White, BorderThickness = new Thickness(0, 0, 0, 0) };
        _primaryBtn.Background = new SolidColorBrush(Color.FromRgb(30, 41, 59));
        _primaryBtn.Effect = SoftShadow();
        _primaryBtn.FontSize = 15;
        _primaryBtn.Click += OnPrimaryClick;
        stack.Children.Add(_primaryBtn);

        var loginRow = new StackPanel { Orientation = Orientation.Horizontal, Margin = new Thickness(0, 20, 0, 0), HorizontalAlignment = HorizontalAlignment.Center };
        loginRow.Children.Add(new TextBlock { Text = "已有账号？ ", FontSize = 13, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), VerticalAlignment = VerticalAlignment.Center });
        var loginBtn = new Button { Content = "登录", Background = Brushes.Transparent, Foreground = new SolidColorBrush(Color.FromRgb(30, 41, 59)), BorderThickness = new Thickness(0, 0, 0, 0), FontWeight = FontWeights.Medium, FontSize = 13, Padding = new Thickness(4, 2, 4, 2), Cursor = Cursors.Hand };
        loginBtn.Click += (s, e) => { DialogResult = false; Close(); };
        loginRow.Children.Add(loginBtn);
        stack.Children.Add(loginRow);

        main.Child = stack;
        Content = main;
    }

    void OnPrimaryClick(object s, RoutedEventArgs e)
    {
        var email = _emailBox.Text?.Trim();
        if (string.IsNullOrEmpty(email)) { ShowError("请输入邮箱"); return; }

        if (_verificationRequired && _step == 1)
        {
            _errorText.Visibility = Visibility.Collapsed;
            _primaryBtn.IsEnabled = false;
            Task.Run(() =>
            {
                try
                {
                    SendVerificationCode(_apiBase, email);
                    Dispatcher.Invoke(() =>
                    {
                        _step = 2;
                        _emailBox.IsEnabled = false;
                        _codePanel.Visibility = Visibility.Visible;
                        _pwdPanel.Visibility = Visibility.Visible;
                        _primaryBtn.Content = "注册";
                        _primaryBtn.IsEnabled = true;
                    });
                }
                catch (Exception ex)
                {
                    Dispatcher.Invoke(() => { ShowError(ex.Message); _primaryBtn.IsEnabled = true; });
                }
            });
            return;
        }

        var password = _pwdBox.Password;
        if (string.IsNullOrEmpty(password)) { ShowError("请输入密码"); return; }
        if (password.Length < 6) { ShowError("密码至少 6 位"); return; }
        if (_verificationRequired)
        {
            var code = _codeBox?.Text?.Trim() ?? "";
            if (string.IsNullOrEmpty(code)) { ShowError("请输入验证码"); return; }
        }

        _errorText.Visibility = Visibility.Collapsed;
        _primaryBtn.IsEnabled = false;
        var api = _apiBase;
        var codeVal = _verificationRequired ? _codeBox.Text?.Trim() : null;
        Task.Run(() =>
        {
            try
            {
                var t = DoRegister(api, email, password, codeVal);
                Dispatcher.Invoke(() =>
                {
                    Token = t;
                    DialogResult = true;
                    Close();
                });
            }
            catch (Exception ex)
            {
                Dispatcher.Invoke(() =>
                {
                    ShowError(ex.Message);
                    _primaryBtn.IsEnabled = true;
                });
            }
        });
    }

    void ShowError(string msg)
    {
        _errorText.Text = msg;
        _errorText.Visibility = Visibility.Visible;
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

static bool GetAuthConfigVerificationRequired(string apiBase)
{
    var (ver, _) = GetAuthConfig(apiBase);
    return ver;
}
static (bool VerificationRequired, int AdoptCost) GetAuthConfig(string apiBase)
{
    try
    {
        var api = apiBase.TrimEnd('/');
        using var client = CreateHttpClient();
        var resp = client.GetAsync(api + "/auth/config").GetAwaiter().GetResult();
        var text = resp.Content.ReadAsStringAsync().GetAwaiter().GetResult();
        if (!resp.IsSuccessStatusCode) return (true, 100);
        var verMatch = Regex.Match(text, @"""email_verification_required""\s*:\s*(true|false)");
        var verificationRequired = !verMatch.Success || verMatch.Groups[1].Value == "true";
        var costMatch = Regex.Match(text, @"""adopt_cost""\s*:\s*(\d+)");
        var adoptCost = costMatch.Success && int.TryParse(costMatch.Groups[1].Value, out var c) && c > 0 ? c : 0;
        if (adoptCost <= 0)
        {
            var er = client.GetAsync(api + "/energy/config").GetAwaiter().GetResult();
            if (er.IsSuccessStatusCode)
            {
                var et = er.Content.ReadAsStringAsync().GetAwaiter().GetResult();
                var em = Regex.Match(et, @"""adopt_cost""\s*:\s*(\d+)");
                if (em.Success && int.TryParse(em.Groups[1].Value, out var ec) && ec > 0) adoptCost = ec;
            }
        }
        if (adoptCost <= 0) adoptCost = 100;
        return (verificationRequired, adoptCost);
    }
    catch { return (true, 100); }
}

static void SendVerificationCode(string apiBase, string email)
{
    using var client = CreateHttpClient();
    var body = "{\"email\":\"" + JsonEscape(email) + "\"}";
    var resp = client.PostAsync(apiBase.TrimEnd('/') + "/auth/send-code", new StringContent(body, Encoding.UTF8, "application/json")).GetAwaiter().GetResult();
    var text = resp.Content.ReadAsStringAsync().GetAwaiter().GetResult();
    if (!resp.IsSuccessStatusCode)
    {
        var m = Regex.Match(text, @"""error""\s*:\s*""([^""]*)""");
        throw new Exception(m.Success ? m.Groups[1].Value : text);
    }
}

static string DoRegister(string apiBase, string email, string password, string code = null)
{
    using var client = CreateHttpClient();
    var sb = new StringBuilder();
    sb.Append("{\"email\":\"").Append(JsonEscape(email)).Append("\",\"password\":\"").Append(JsonEscape(password)).Append("\"");
    if (!string.IsNullOrEmpty(code)) sb.Append(",\"code\":\"").Append(JsonEscape(code)).Append("\"");
    sb.Append("}");
    var resp = client.PostAsync(apiBase.TrimEnd('/') + "/auth/register", new StringContent(sb.ToString(), Encoding.UTF8, "application/json")).GetAwaiter().GetResult();
    var text = resp.Content.ReadAsStringAsync().GetAwaiter().GetResult();
    if (!resp.IsSuccessStatusCode)
    {
        var m = Regex.Match(text, @"""error""\s*:\s*""([^""]*)""");
        throw new Exception(m.Success ? m.Groups[1].Value : text);
    }
    var tokMatch = Regex.Match(text, @"""access_token""\s*:\s*""([^""]+)""");
    if (tokMatch.Success) return tokMatch.Groups[1].Value;
    throw new Exception("注册响应无 access_token");
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
    var adoptMatch = Regex.Match(text, @"""adopt_cost""\s*:\s*(\d+)");
    var adoptCost = adoptMatch.Success && int.TryParse(adoptMatch.Groups[1].Value, out var ac) && ac > 0 ? ac : 0;
    return new UserInfo { Energy = energyMatch.Success ? int.Parse(energyMatch.Groups[1].Value) : 0, Email = emailMatch.Success ? emailMatch.Groups[1].Value : "", AdoptCost = adoptCost };
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
        // 用正则逐条匹配消息对象，保持 API 返回顺序（id DESC），避免 },{ 分割破坏 content
        var objRegex = new Regex(@"\{\s*""id""\s*:\s*(\d+)[^}]*""role""\s*:\s*""([^""]*)""[^}]*""content""\s*:\s*""((?:[^""\\]|\\.)*)""", RegexOptions.Singleline);
        foreach (Match m in objRegex.Matches(text))
        {
            var id = m.Groups[1].Value;
            var role = m.Groups[2].Value;
            var content = UnescapeJsonString(m.Groups[3].Value ?? "");
            if (string.IsNullOrEmpty(content) || content.StartsWith("Thinking")) continue;
            if (string.IsNullOrEmpty(role)) role = "assistant";
            list.Add((id, content, role == "user"));
        }
        // 按 id 升序排成 一问一答 顺序（旧→新）
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

// 解析 WebSocket 消息（正则，兼容 Quicker Roslyn 无 System.Text.Json）
static (string type, string content, string role, string messageId) ParseWsMessage(string json)
{
    if (string.IsNullOrEmpty(json)) return ("", "", "", "");
    var typeMatch = Regex.Match(json, "\"type\"\\s*:\\s*\"([^\"]+)\"");
    var msgIdMatch = Regex.Match(json, "\"message_id\"\\s*:\\s*\"?([\\d\\w-]*)\"?");
    var roleMatch = Regex.Match(json, "\"role\"\\s*:\\s*\"([^\"]+)\"");
    var content = "";
    var contentMatch = Regex.Match(json, "\"content\"\\s*:\\s*\"((?:[^\"\\\\]|\\\\\\\\.)*)\"");
    if (contentMatch.Success) content = UnescapeJsonString(contentMatch.Groups[1].Value);
    else
    {
        var fallback = Regex.Match(json, "\"content\"\\s*:\\s*\"([^\"]*)\"");
        if (fallback.Success) content = UnescapeJsonString(fallback.Groups[1].Value);
    }
    return (
        typeMatch.Success ? typeMatch.Groups[1].Value : "",
        content,
        roleMatch.Success ? roleMatch.Groups[1].Value : "",
        msgIdMatch.Success ? msgIdMatch.Groups[1].Value : ""
    );
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

class UserInfo { public int Energy; public string Email; public int AdoptCost; }
class RechargePlan { public string Id; public string Name; public string Benefits; public int Energy; public int PriceCny; }

static List<RechargePlan> GetRechargePlans(string apiBase, string token)
{
    var list = new List<RechargePlan>();
    try
    {
        using var client = CreateHttpClient();
        client.DefaultRequestHeaders.Add("Authorization", "Bearer " + token);
        var resp = client.GetAsync(apiBase.TrimEnd('/') + "/energy/recharge/plans").GetAwaiter().GetResult();
        var text = resp.Content.ReadAsStringAsync().GetAwaiter().GetResult();
        if (!resp.IsSuccessStatusCode) return list;
        var parts = text.Split(new[] { "},{" }, StringSplitOptions.None);
        foreach (var p in parts)
        {
            var block = p.Trim().TrimStart('[').TrimEnd(']').TrimStart('{').TrimEnd('}');
            if (string.IsNullOrEmpty(block)) continue;
            var idMatch = Regex.Match(block, @"""id""\s*:\s*""([^""]*)""");
            var nameMatch = Regex.Match(block, @"""name""\s*:\s*""([^""]*)""");
            var benefitsMatch = Regex.Match(block, @"""benefits""\s*:\s*""([^""]*)""");
            var energyMatch = Regex.Match(block, @"""energy""\s*:\s*(\d+)");
            var priceMatch = Regex.Match(block, @"""price_cny""\s*:\s*(\d+)");
            if (nameMatch.Success && energyMatch.Success && int.TryParse(energyMatch.Groups[1].Value, out var energy) && priceMatch.Success && int.TryParse(priceMatch.Groups[1].Value, out var price))
            {
                var benefits = benefitsMatch.Success ? benefitsMatch.Groups[1].Value : (energy + " 金币");
                list.Add(new RechargePlan { Id = idMatch.Success ? idMatch.Groups[1].Value : "", Name = nameMatch.Groups[1].Value, Benefits = benefits, Energy = energy, PriceCny = price });
            }
        }
    }
    catch { }
    return list;
}
class InstanceItem { public int Id; public string Name; public string Status; }

static readonly List<ClawMascotWindow> s_mascots = new List<ClawMascotWindow>();

class ClawAdminWindow : Window
{
    readonly string _apiBase;
    readonly string _token;
    Point _dragOffset;
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
        var tb = new TextBlock { Text = "🏠", FontSize = 36, VerticalAlignment = VerticalAlignment.Center, HorizontalAlignment = HorizontalAlignment.Center };
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

        canvas.MouseLeftButtonDown += (s, e) => { _dragOffset = e.GetPosition(this); _dragStartScreen = PointToScreen(_dragOffset); canvas.CaptureMouse(); };
        canvas.MouseLeftButtonUp += (s, e) =>
        {
            canvas.ReleaseMouseCapture();
            var p = PointToScreen(e.GetPosition(this));
            if (Math.Abs(p.X - _dragStartScreen.X) < 4 && Math.Abs(p.Y - _dragStartScreen.Y) < 4)
            {
                var main = new AnyClawMainWindow(_apiBase, _token);
                main.Owner = this;
                main.ShowDialog();
            }
        };
        canvas.MouseMove += (s, e) =>
        {
            if (e.LeftButton == MouseButtonState.Pressed)
            {
                var p = PointToScreen(e.GetPosition(this));
                Left = p.X - _dragOffset.X;
                Top = p.Y - _dragOffset.Y;
            }
        };
        Content = canvas;
    }
}

class ClawMascotWindow : Window
{
    readonly string _apiBase;
    readonly string _token;
    readonly int _instanceId;
    readonly string _instanceName;
    Point _dragOffset;
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
            BorderThickness = new Thickness(2, 2, 2, 2),
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
        _canvas.MouseLeftButtonDown += (s, e) => { _dragOffset = e.GetPosition(this); _dragStartScreen = PointToScreen(_dragOffset); _canvas.CaptureMouse(); };
        _canvas.MouseLeftButtonUp += OnMouseLeftButtonUp;
        _canvas.MouseMove += (s, e) =>
        {
            if (e.LeftButton == MouseButtonState.Pressed)
            {
                var p = PointToScreen(e.GetPosition(this));
                Left = p.X - _dragOffset.X;
                Top = p.Y - _dragOffset.Y;
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
        if (_hasUnread)
        {
            _notificationBadge.Background = new SolidColorBrush(Color.FromRgb(239, 68, 68));
            _badgeText.Text = "1";
            _notificationBadge.Visibility = Visibility.Visible;
        }
        else
        {
            _notificationBadge.Visibility = Visibility.Collapsed;
        }
    }

    void ShowQuickReply()
    {
        // 如果窗口已打开，先关闭
        if (_quickReplyWindow != null && _quickReplyWindow.IsVisible)
        {
            _quickReplyWindow.Close();
        }

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
            var mdTextBlock = CreateMarkdownTextBlock(_lastMessageContent, false);
            mdTextBlock.FontSize = 13;
            mdTextBlock.MaxWidth = 290;
            scrollViewer.Content = mdTextBlock;
            sp.Children.Add(scrollViewer);
        }
        else
        {
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
            BorderThickness = new Thickness(1, 1, 1, 1)
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
            BorderThickness = new Thickness(0, 0, 0, 0)
        };
        sendBtn.Click += (s, e) => SendQuickReply();

        inputRow.Children.Add(_quickReplyBox);
        inputRow.Children.Add(sendBtn);

        sp.Children.Add(inputRow);
        border.Child = sp;
        _quickReplyWindow.Content = border;

        _quickReplyWindow.Deactivated += (s, e) => _quickReplyWindow?.Close();

        _quickReplyBox.Text = "";
        _quickReplyWindow.Show();
        _quickReplyBox.Focus();
    }

    void SendQuickReply()
    {
        var text = _quickReplyBox.Text?.Trim();
        if (string.IsNullOrEmpty(text)) return;

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
        while (_ws != null && _ws.State == WebSocketState.Open && !_cts.Token.IsCancellationRequested)
        {
            try
            {
                var sb = new StringBuilder();
                WebSocketReceiveResult result = null;
                do
                {
                    result = await _ws.ReceiveAsync(new ArraySegment<byte>(buf), _cts.Token);

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

                var (type, content, role, msgIdStr) = ParseWsMessage(json);

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
            catch { /* 单条消息异常不退出，继续接收 */ }
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

class RechargeWindow : Window
{
    readonly string _apiBase;
    readonly string _token;

    public RechargeWindow(string apiBase, string token, string currentEnergy)
    {
        _apiBase = apiBase;
        _token = token;
        Title = "充值金币";
        Width = 420;
        Height = 580;
        MinHeight = 450;
        WindowStartupLocation = WindowStartupLocation.CenterOwner;
        Background = GradBg();

        var main = new ScrollViewer { VerticalScrollBarVisibility = ScrollBarVisibility.Auto, Padding = new Thickness(0, 0, 0, 0) };
        var stack = new StackPanel { Margin = new Thickness(20, 20, 20, 24) };

        var headerCard = new Border { Padding = new Thickness(20, 16, 20, 16), Margin = new Thickness(0, 0, 0, 16), CornerRadius = new CornerRadius(16), Effect = CardShadow() };
        headerCard.Background = new LinearGradientBrush(Color.FromRgb(255, 251, 235), Color.FromRgb(254, 243, 199), new Point(0, 0), new Point(1, 1));
        headerCard.BorderBrush = new SolidColorBrush(Color.FromRgb(253, 230, 138));
        headerCard.BorderThickness = new Thickness(2, 2, 2, 2);
        var headerStack = new StackPanel();
        var titleRow = new StackPanel { Orientation = Orientation.Horizontal };
        var coinIcon = new Viewbox { Width = 28, Height = 28, Margin = new Thickness(0, 0, 8, 0) };
        var coinCanvas = new Canvas { Width = 24, Height = 24 };
        var coin = new Ellipse { Width = 20, Height = 20, Fill = new SolidColorBrush(Color.FromRgb(251, 191, 36)), Stroke = new SolidColorBrush(Color.FromRgb(217, 119, 6)), StrokeThickness = 1.5 };
        Canvas.SetLeft(coin, 2);
        Canvas.SetTop(coin, 2);
        coinCanvas.Children.Add(coin);
        coinIcon.Child = coinCanvas;
        titleRow.Children.Add(coinIcon);
        titleRow.Children.Add(new TextBlock { Text = "充值金币", FontSize = 22, FontWeight = FontWeights.Bold, Foreground = new SolidColorBrush(Color.FromRgb(30, 41, 59)), VerticalAlignment = VerticalAlignment.Center });
        headerStack.Children.Add(titleRow);
        headerStack.Children.Add(new TextBlock { Text = "当前余额：", FontSize = 14, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), Margin = new Thickness(0, 4, 0, 0) });
        headerStack.Children.Add(new TextBlock { Text = (currentEnergy ?? "0") + " 金币", FontSize = 18, FontWeight = FontWeights.Bold, Foreground = new SolidColorBrush(Color.FromRgb(217, 119, 6)), Margin = new Thickness(0, 2, 0, 0) });
        headerCard.Child = headerStack;
        stack.Children.Add(headerCard);

        var qrCard = new Border { Background = Brushes.White, Padding = new Thickness(20, 16, 20, 16), Margin = new Thickness(0, 0, 0, 16), CornerRadius = new CornerRadius(12), Effect = SoftShadow(), BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240)), BorderThickness = new Thickness(1, 1, 1, 1) };
        var qrStack = new StackPanel();
        qrStack.Children.Add(new TextBlock { Text = "推荐使用微信支付", FontSize = 14, FontWeight = FontWeights.Medium, Foreground = new SolidColorBrush(Color.FromRgb(51, 65, 85)), Margin = new Thickness(0, 0, 0, 12) });
        var imgContainer = new Border { Width = 240, Height = 240, Background = new SolidColorBrush(Color.FromRgb(248, 250, 252)), CornerRadius = new CornerRadius(8), Margin = new Thickness(0, 0, 0, 12) };
        try
        {
            var uri = new Uri(_apiBase.TrimEnd('/') + "/pay_compressed.png");
            var bmp = new BitmapImage();
            bmp.BeginInit();
            bmp.UriSource = uri;
            bmp.CacheOption = BitmapCacheOption.OnLoad;
            bmp.EndInit();
            imgContainer.Child = new Image { Source = bmp, Stretch = Stretch.Uniform };
        }
        catch { imgContainer.Child = new TextBlock { Text = "二维码加载失败", HorizontalAlignment = HorizontalAlignment.Center, VerticalAlignment = VerticalAlignment.Center, Foreground = new SolidColorBrush(Color.FromRgb(148, 163, 184)) }; }
        qrStack.Children.Add(imgContainer);
        var saveBtn = new Button { Content = "保存图片", Width = 120, Height = 40, FontSize = 13 };
        saveBtn.Background = new SolidColorBrush(Color.FromRgb(5, 150, 105));
        saveBtn.Foreground = Brushes.White;
        saveBtn.BorderThickness = new Thickness(0, 0, 0, 0);
        saveBtn.Click += (s, e) =>
        {
            try
            {
                using var client = CreateHttpClient();
                var bytes = client.GetByteArrayAsync(_apiBase.TrimEnd('/') + "/pay_compressed.png").GetAwaiter().GetResult();
                var path = System.IO.Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.Desktop), "微信支付二维码.png");
                File.WriteAllBytes(path, bytes);
                MessageBox.Show("已保存到桌面：微信支付二维码.png");
            }
            catch (Exception ex) { MessageBox.Show("保存失败: " + ex.Message); }
        };
        qrStack.Children.Add(saveBtn);
        qrStack.Children.Add(new TextBlock { Text = "扫码付款，请务必在备注中填写您的注册邮箱，人工审核成功后金币自动到账", FontSize = 12, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), Margin = new Thickness(0, 12, 0, 0), TextWrapping = TextWrapping.Wrap });
        qrCard.Child = qrStack;
        stack.Children.Add(qrCard);

        var plans = GetRechargePlans(_apiBase, _token);
        if (plans.Count > 0)
        {
            stack.Children.Add(new TextBlock { Text = "充值档位参考", FontSize = 14, FontWeight = FontWeights.Medium, Foreground = new SolidColorBrush(Color.FromRgb(51, 65, 85)), Margin = new Thickness(0, 0, 0, 8) });
            foreach (var p in plans)
            {
                var planCard = new Border { Padding = new Thickness(16, 12, 16, 12), Margin = new Thickness(0, 0, 0, 8), Background = Brushes.White, CornerRadius = new CornerRadius(10), Effect = SoftShadow(), BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240)), BorderThickness = new Thickness(1, 1, 1, 1) };
                var planStack = new StackPanel();
                planStack.Children.Add(new TextBlock { Text = p.Name, FontSize = 15, FontWeight = FontWeights.Medium, Foreground = new SolidColorBrush(Color.FromRgb(30, 41, 59)) });
                planStack.Children.Add(new TextBlock { Text = p.Benefits ?? (p.Energy + " 金币"), FontSize = 13, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), Margin = new Thickness(0, 2, 0, 0) });
                planStack.Children.Add(new TextBlock { Text = "¥" + (p.PriceCny / 100.0).ToString("F2"), FontSize = 14, FontWeight = FontWeights.Medium, Foreground = new SolidColorBrush(Color.FromRgb(217, 119, 6)), Margin = new Thickness(0, 4, 0, 0) });
                planCard.Child = planStack;
                stack.Children.Add(planCard);
            }
        }

        var customCard = new Border { Padding = new Thickness(16, 14, 16, 14), Margin = new Thickness(0, 8, 0, 0), Background = new SolidColorBrush(Color.FromRgb(248, 250, 252)), CornerRadius = new CornerRadius(10), BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240)), BorderThickness = new Thickness(1, 1, 1, 1) };
        var customStack = new StackPanel();
        customStack.Children.Add(new TextBlock { Text = "任意金额充值", FontSize = 14, FontWeight = FontWeights.Medium, Foreground = new SolidColorBrush(Color.FromRgb(51, 65, 85)) });
        customStack.Children.Add(new TextBlock { Text = "支持任意金额充值（≥10 元），付款时请在备注中填写您的注册邮箱，管理员审核通过后金币将自动到账。", FontSize = 13, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), Margin = new Thickness(0, 6, 0, 0), TextWrapping = TextWrapping.Wrap });
        customCard.Child = customStack;
        stack.Children.Add(customCard);

        main.Content = stack;
        Content = main;
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
    readonly Dictionary<int, CheckBox> _showChecks = new Dictionary<int, CheckBox>();
    readonly Dictionary<int, ComboBox> _schemeCombos = new Dictionary<int, ComboBox>();

    public AnyClawMainWindow(string apiBase, string token)
    {
        _apiBase = apiBase;
        _token = token;
        Title = "OpenClaw 宠舍";
        Width = 440;
        Height = 560;
        MinHeight = 480;
        WindowStartupLocation = WindowStartupLocation.CenterScreen;
        Background = GradBg();

        var grid = new Grid { Margin = new Thickness(20, 20, 20, 20) };
        grid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        grid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        grid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        grid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        grid.RowDefinitions.Add(new RowDefinition { Height = new GridLength(1, GridUnitType.Star) });
        grid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });

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
        Grid.SetRow(header, 0);
        grid.Children.Add(header);

        var energyPanel = new Border { Padding = new Thickness(20, 14, 20, 14), Margin = new Thickness(0, 20, 0, 0), CornerRadius = new CornerRadius(16), Effect = CardShadow() };
        energyPanel.Background = GoldGrad();
        var energyRow = new Grid();
        energyRow.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        energyRow.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        var energyLeft = new StackPanel { Orientation = Orientation.Horizontal };
        var coinIcon = new Viewbox { Width = 24, Height = 24, Margin = new Thickness(0, 0, 10, 0) };
        var coinCanvas = new Canvas { Width = 24, Height = 24 };
        var coin = new Ellipse { Width = 20, Height = 20, Fill = new SolidColorBrush(Color.FromRgb(251, 191, 36)), Stroke = new SolidColorBrush(Color.FromRgb(217, 119, 6)), StrokeThickness = 1.5 };
        Canvas.SetLeft(coin, 2);
        Canvas.SetTop(coin, 2);
        coinCanvas.Children.Add(coin);
        coinIcon.Child = coinCanvas;
        energyLeft.Children.Add(coinIcon);
        energyLeft.Children.Add(new TextBlock { Text = "我的金币 ", VerticalAlignment = VerticalAlignment.Center, FontSize = 15, Foreground = new SolidColorBrush(Color.FromRgb(120, 53, 15)) });
        _energyText = new TextBlock { Text = "0", FontWeight = FontWeights.Bold, FontSize = 22, Foreground = new SolidColorBrush(Color.FromRgb(120, 53, 15)), VerticalAlignment = VerticalAlignment.Center };
        energyLeft.Children.Add(_energyText);
        Grid.SetColumn(energyLeft, 0);
        energyRow.Children.Add(energyLeft);
        var rechargeBtn = new Button { Content = "充值", Width = 70, Height = 32, FontSize = 13 };
        rechargeBtn.Background = new SolidColorBrush(Color.FromRgb(217, 119, 6));
        rechargeBtn.Foreground = Brushes.White;
        rechargeBtn.BorderThickness = new Thickness(0, 0, 0, 0);
        rechargeBtn.Click += (s, e) =>
        {
            var win = new RechargeWindow(_apiBase, _token, _energyText.Text ?? "0");
            win.Owner = this;
            win.Closed += (s2, e2) => Refresh();
            win.ShowDialog();
        };
        Grid.SetColumn(rechargeBtn, 1);
        energyRow.Children.Add(rechargeBtn);
        energyPanel.Child = energyRow;
        Grid.SetRow(energyPanel, 1);
        grid.Children.Add(energyPanel);

        var adoptCard = new Border { Background = Brushes.White, Padding = new Thickness(20, 16, 20, 16), Margin = new Thickness(0, 16, 0, 0), CornerRadius = new CornerRadius(16), Effect = CardShadow() };
        var adoptPanel = new StackPanel();
        adoptPanel.Children.Add(new TextBlock { Text = "领养 OpenClaw", FontSize = 16, FontWeight = FontWeights.SemiBold, Foreground = new SolidColorBrush(Color.FromRgb(15, 23, 42)) });
        adoptPanel.Children.Add(new TextBlock { Text = "每只宠物都有唯一的灵魂，擅长复杂任务、拥有超长记忆", FontSize = 12, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), Margin = new Thickness(0, 6, 0, 0) });
        var adoptRow = new StackPanel { Orientation = Orientation.Horizontal, Margin = new Thickness(0, 12, 0, 0) };
        _newNameBox = new TextBox { Width = 170, Height = 40, Padding = new Thickness(14, 10, 14, 10), VerticalContentAlignment = VerticalAlignment.Center, FontSize = 14 };
        _newNameBox.BorderBrush = new SolidColorBrush(Color.FromRgb(226, 232, 240));
        _newNameBox.BorderThickness = new Thickness(1, 1, 1, 1);
        _adoptBtn = new Button { Content = "领养", Width = 140, Height = 40, Margin = new Thickness(14, 0, 0, 0), FontSize = 14, Foreground = Brushes.White, BorderThickness = new Thickness(0, 0, 0, 0) };
        _adoptBtn.Background = AccentGrad();
        _adoptBtn.Effect = SoftShadow();
        _adoptBtn.Click += OnAdopt;
        adoptRow.Children.Add(_newNameBox);
        adoptRow.Children.Add(_adoptBtn);
        adoptPanel.Children.Add(adoptRow);
        adoptCard.Child = adoptPanel;
        Grid.SetRow(adoptCard, 2);
        grid.Children.Add(adoptCard);

        var petHeader = new StackPanel { Margin = new Thickness(0, 24, 0, 0) };
        petHeader.Children.Add(new TextBlock { Text = "我的宠舍", FontSize = 16, FontWeight = FontWeights.SemiBold, Foreground = new SolidColorBrush(Color.FromRgb(15, 23, 42)) });
        petHeader.Children.Add(new TextBlock { Text = "勾选要显示的宠物、选择配色，点击「应用到桌面」。右键可弃养，双击可打开对话。", FontSize = 12, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), Margin = new Thickness(0, 4, 0, 0), TextWrapping = TextWrapping.Wrap });
        Grid.SetRow(petHeader, 3);
        grid.Children.Add(petHeader);

        _instancePanel = new StackPanel();
        _instanceScroll = new ScrollViewer { Content = _instancePanel, VerticalScrollBarVisibility = ScrollBarVisibility.Auto, Padding = new Thickness(2), Margin = new Thickness(0, 8, 0, 0) };
        Grid.SetRow(_instanceScroll, 4);
        grid.Children.Add(_instanceScroll);

        var applyBtn = new Button { Content = "应用到桌面", Width = 140, Height = 42, Margin = new Thickness(0, 12, 0, 0), FontSize = 14, Foreground = Brushes.White, BorderThickness = new Thickness(0, 0, 0, 0) };
        applyBtn.Background = AccentGrad();
        applyBtn.Effect = SoftShadow();
        applyBtn.Click += OnApply;
        var qqLink = new TextBlock { Text = "加入 OpenClaw 探索群", FontSize = 12, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), Cursor = Cursors.Hand, Margin = new Thickness(0, 8, 0, 0), HorizontalAlignment = HorizontalAlignment.Center };
        qqLink.MouseDown += (s, e) =>
        {
            var win = new Window { Title = "加入 OpenClaw 探索群", Width = 320, Height = 420, WindowStartupLocation = WindowStartupLocation.CenterOwner, Owner = this };
            win.Background = Brushes.White;
            var sp = new StackPanel { Margin = new Thickness(24, 24, 24, 24) };
            sp.Children.Add(new TextBlock { Text = "OpenClaw 探索群 群号: 1049101776", FontSize = 14, FontWeight = FontWeights.Medium, Foreground = new SolidColorBrush(Color.FromRgb(51, 65, 85)), Margin = new Thickness(0, 0, 0, 12) });
            var qqImgBorder = new Border { Width = 200, Height = 200, Background = Brushes.White, CornerRadius = new CornerRadius(8), Effect = SoftShadow() };
            try
            {
                using var client = CreateHttpClient();
                var bytes = client.GetByteArrayAsync(_apiBase.TrimEnd('/') + "/qqgroup.jpg").GetAwaiter().GetResult();
                if (bytes != null && bytes.Length > 0)
                {
                    using var ms = new MemoryStream(bytes);
                    var qqBmp = new BitmapImage();
                    qqBmp.BeginInit();
                    qqBmp.StreamSource = ms;
                    qqBmp.CacheOption = BitmapCacheOption.OnLoad;
                    qqBmp.EndInit();
                    qqBmp.Freeze();
                    qqImgBorder.Child = new Image { Source = qqBmp, Stretch = Stretch.Uniform };
                }
                else throw new Exception("empty");
            }
            catch { qqImgBorder.Child = new TextBlock { Text = "群号\n1049101776\n\n(图片加载失败，请确保 API 已部署最新 Web)", FontSize = 12, HorizontalAlignment = HorizontalAlignment.Center, VerticalAlignment = VerticalAlignment.Center, TextAlignment = TextAlignment.Center, Foreground = new SolidColorBrush(Color.FromRgb(148, 163, 184)), TextWrapping = TextWrapping.Wrap }; }
            sp.Children.Add(qqImgBorder);
            sp.Children.Add(new TextBlock { Text = "扫一扫加入群聊", FontSize = 12, Foreground = new SolidColorBrush(Color.FromRgb(100, 116, 139)), Margin = new Thickness(0, 12, 0, 0), HorizontalAlignment = HorizontalAlignment.Center });
            win.Content = sp;
            win.ShowDialog();
        };
        var row5 = new StackPanel { HorizontalAlignment = HorizontalAlignment.Center };
        row5.Children.Add(applyBtn);
        row5.Children.Add(qqLink);
        Grid.SetRow(row5, 5);
        grid.Children.Add(row5);

        Content = grid;
        Loaded += (s, e) => Refresh();
    }

    void Refresh()
    {
        try
        {
            var user = GetMe(_apiBase, _token);
            var adoptCost = user.AdoptCost > 0 ? user.AdoptCost : 0;
            if (adoptCost <= 0) { var (_, c) = GetAuthConfig(_apiBase); adoptCost = c > 0 ? c : 100; }
            if (adoptCost <= 0) adoptCost = 100;
            _adoptBtn.Content = "领养 · " + adoptCost + " 金币";
            _energyText.Text = user.Energy.ToString();
            var instances = GetInstances(_apiBase, _token);
            var saved = LoadDesktopConfig(_apiBase);
            var savedDict = saved.ToDictionary(x => x.Id, x => x.SchemeId);
            _instancePanel.Children.Clear();
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
                var statusText = i.Status == "running" ? "在线" : i.Status == "creating" ? "创建中" : i.Status == "error" ? "异常" : i.Status;
                var statusColor = i.Status == "running" ? Color.FromRgb(34, 197, 94) : i.Status == "creating" ? Color.FromRgb(245, 158, 11) : Color.FromRgb(239, 68, 68);
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
                var cardBorder = new Border { Child = row, Padding = new Thickness(14, 12, 14, 12), Margin = new Thickness(0, 0, 0, 10), Background = Brushes.White, CornerRadius = new CornerRadius(10), Effect = SoftShadow(), Cursor = Cursors.Hand };
                var card = new ContentControl { Content = cardBorder, Margin = new Thickness(0, 0, 0, 0), Cursor = Cursors.Hand };
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
            _adoptBtn.IsEnabled = user.Energy >= adoptCost;
        }
        catch (Exception ex)
        {
            MessageBox.Show("刷新失败: " + ex.Message);
        }
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
            MessageBox.Show("已应用到桌面");
        }
        catch (Exception ex) { MessageBox.Show("应用失败: " + ex.Message); }
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
        _scrollViewer = new ScrollViewer { Content = msgStack, VerticalScrollBarVisibility = ScrollBarVisibility.Auto, Padding = new Thickness(0, 0, 0, 0) };
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
            BorderThickness = new Thickness(0, 0, 0, 0),
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
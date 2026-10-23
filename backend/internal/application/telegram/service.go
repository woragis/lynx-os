package telegram

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	identitydomain "github.com/lynx/lynx-os/backend/internal/domain/identity"
	integrationdomain "github.com/lynx/lynx-os/backend/internal/domain/integration"
	domain "github.com/lynx/lynx-os/backend/internal/domain/telegram"
)

var ErrAIUnavailable = errors.New("telegram assistant could not organize the message")

type SecretCipher interface {
	Encrypt([]byte) (integrationdomain.SealedSecret, error)
	Decrypt(integrationdomain.SealedSecret) ([]byte, error)
}

type Service struct {
	conversations domain.ConversationRepository
	identities    domain.IdentityRepository
	linker        domain.IdentityLinker
	integrations  domain.IntegrationGateway
	extractor     domain.DraftExtractor
	cipher        SecretCipher
	now           func() time.Time
}

func NewService(conversations domain.ConversationRepository, identities domain.IdentityRepository, linker domain.IdentityLinker,
	integrations domain.IntegrationGateway, extractor domain.DraftExtractor, cipher SecretCipher) *Service {
	return &Service{conversations: conversations, identities: identities, linker: linker, integrations: integrations, extractor: extractor, cipher: cipher, now: time.Now}
}

func (s *Service) BeginUpdate(ctx context.Context, updateID int64, chatID string) (bool, error) {
	return s.conversations.BeginUpdate(ctx, updateID, chatID)
}

func (s *Service) CompleteUpdate(ctx context.Context, updateID int64, chatID string, ignored bool) error {
	return s.conversations.CompleteUpdate(ctx, updateID, chatID, ignored)
}

func (s *Service) UpdateOffset(ctx context.Context) (int64, error) {
	return s.conversations.GetUpdateOffset(ctx)
}

func (s *Service) SetBotMessage(ctx context.Context, telegramID string, messageID int64) error {
	conversation, sealed, err := s.conversations.GetConversation(ctx, telegramID)
	if err != nil {
		return err
	}
	conversation.BotMessageID = &messageID
	return s.conversations.SaveConversation(ctx, conversation, sealed)
}

func (s *Service) HandleUpdate(ctx context.Context, update domain.Update) (*domain.Reply, error) {
	var telegramID, text string
	var chatID int64
	var chatType string
	var callbackID string
	if update.Message != nil {
		telegramID = strconv.FormatInt(update.Message.FromID, 10)
		chatID, chatType, text = update.Message.ChatID, update.Message.ChatType, strings.TrimSpace(update.Message.Text)
	} else if update.Callback != nil {
		telegramID = strconv.FormatInt(update.Callback.FromID, 10)
		chatID, chatType, text, callbackID = update.Callback.ChatID, update.Callback.ChatType, strings.TrimSpace(update.Callback.Data), update.Callback.ID
	} else {
		return nil, nil
	}
	if telegramID == "0" || chatID == 0 || chatType != "private" || telegramID != strconv.FormatInt(chatID, 10) {
		return nil, nil
	}

	if strings.HasPrefix(text, "/link ") {
		code := strings.TrimSpace(strings.TrimPrefix(text, "/link "))
		if err := s.linker.LinkTelegramIdentity(ctx, telegramID, code); err != nil {
			conversation, data, loadErr := s.loadConversation(ctx, telegramID)
			if loadErr != nil {
				return nil, loadErr
			}
			return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Código inválido ou expirado. Gere outro na área Segurança do Lynx OS."}, "")
		}
		user, err := s.identities.FindByTelegramUserID(ctx, telegramID)
		if err != nil {
			return nil, err
		}
		conversation, _, err := s.loadConversation(ctx, telegramID)
		if err != nil {
			return nil, err
		}
		conversation.State = "MENU"
		return s.saveReply(ctx, conversation, domain.ConversationData{}, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Telegram vinculado. Escolha uma ação:", Buttons: menuButtons()}, user.ID)
	}
	user, err := s.identities.FindByTelegramUserID(ctx, telegramID)
	if err != nil {
		conversation, data, loadErr := s.loadConversation(ctx, telegramID)
		if loadErr != nil {
			return nil, loadErr
		}
		return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Para usar o Lynx AI Assistant, entre em Segurança no Lynx OS, gere um código e envie /link CÓDIGO aqui."}, "")
	}
	if !user.Active {
		return nil, nil
	}

	conversation, data, err := s.loadConversation(ctx, telegramID)
	if err != nil {
		return nil, err
	}
	if conversation.AdminUserID != "" && conversation.AdminUserID != user.ID {
		data = domain.ConversationData{}
		conversation.State = "MENU"
	}
	if isStart(text) {
		if data.PendingOperation != "" && conversation.State == "AWAITING_CONFIRMATION" {
			message := "Há uma alteração pendente: " + data.PendingOperation + "."
			buttons := [][]domain.Button{{{Text: "Confirmar", Data: "confirm"}, {Text: "Cancelar", Data: "menu:cancel"}}}
			if data.PendingOperation == "resource.purge" {
				message += " Digite PURGAR para confirmar a ação permanente."
				buttons = [][]domain.Button{{{Text: "Cancelar", Data: "menu:cancel"}}}
			}
			return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: message, Buttons: buttons}, user.ID)
		}
		if conversation.State == "AWAITING_CONFIRMATION" && data.Draft.UserEmail != nil {
			selected, selectErr := s.activeIntegration(ctx, data.IntegrationID)
			if selectErr != nil {
				return nil, selectErr
			}
			return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: formatPreview(data.Draft, selected.Name), Buttons: [][]domain.Button{{{Text: "Confirmar cadastro", Data: "confirm"}, {Text: "Cancelar", Data: "menu:cancel"}}}}, user.ID)
		}
		if conversation.State == "COLLECTING" {
			return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Seu rascunho continua salvo. Envie os próximos dados em linguagem natural; use Cancelar para descartar.", Buttons: [][]domain.Button{{{Text: "Cancelar rascunho", Data: "menu:cancel"}}}}, user.ID)
		}
		conversation.State = "MENU"
		data = domain.ConversationData{}
		reply := menuReply(chatID, callbackID, "Lynx OS pronto. Escolha uma ação; posso coletar um cadastro em linguagem natural e pedir confirmação antes de enviar.")
		return s.saveReply(ctx, conversation, data, reply, user.ID)
	}
	if text == "/cancelar" || strings.EqualFold(text, "cancelar") || text == "menu:cancel" {
		conversation.State = "MENU"
		data = domain.ConversationData{}
		return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Ação cancelada. Escolha uma opção:", Buttons: menuButtons()}, user.ID)
	}
	if !user.Role.Allows(identitydomain.PermissionViewIntegrations) {
		return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Sua conta não tem acesso aos sistemas Lynx."}, nil
	}

	if update.Callback != nil {
		return s.handleCallback(ctx, user.ID, chatID, callbackID, text, conversation, data)
	}
	if data.PendingOperation != "" && conversation.State == "AWAITING_CONFIRMATION" {
		return s.confirm(ctx, user.ID, telegramID, chatID, callbackID, text, conversation, data)
	}
	if conversation.State == "EDITING_RESOURCE" {
		return s.collectResourceChanges(ctx, user.ID, chatID, callbackID, text, conversation, data)
	}
	if conversation.State == "COLLECTING" {
		return s.collectProvisioning(ctx, user.ID, telegramID, chatID, callbackID, text, conversation, data)
	}
	return s.saveReply(ctx, conversation, data, menuReply(chatID, callbackID, "Escolha Novo cadastro ou Listar empresas, clientes e usuários."), user.ID)
}

func (s *Service) handleCallback(ctx context.Context, userID string, chatID int64, callbackID, dataValue string, conversation domain.Conversation, data domain.ConversationData) (*domain.Reply, error) {
	switch {
	case dataValue == "menu:new":
		if !s.userRoleAllows(ctx, userID, identitydomain.PermissionRunOperations) {
			return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Seu papel é somente de leitura."}, nil
		}
		conversation.State = "COLLECTING"
		data = domain.ConversationData{}
		r := &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Descreva em frases simples o sistema, o escritório, o cliente e a pessoa que terá acesso. Vou perguntar apenas os dados que faltarem."}
		return s.saveReply(ctx, conversation, data, r, userID)
	case strings.HasPrefix(dataValue, "menu:list:"):
		resourceType := strings.TrimPrefix(dataValue, "menu:list:")
		if !validResourceType(resourceType) {
			return nil, nil
		}
		data = domain.ConversationData{ResourceType: resourceType}
		conversation.State = "SELECTING_RESOURCE"
		systems, err := s.integrationSystems(ctx)
		if err != nil {
			return nil, err
		}
		if len(systems) == 1 {
			data.IntegrationID = systems[0].ID
			if resourceType == "account" {
				return s.listResources(ctx, userID, chatID, callbackID, conversation, data, "")
			}
			return s.chooseAccount(ctx, userID, chatID, callbackID, conversation, data)
		}
		buttons := [][]domain.Button{}
		for _, system := range systems {
			buttons = append(buttons, []domain.Button{{Text: system.Name, Data: "system:" + system.ID + ":" + resourceType}})
		}
		if len(buttons) == 0 {
			return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Nenhum SaaS ativo está conectado."}, nil
		}
		return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Escolha o SaaS:", Buttons: buttons}, userID)
	case strings.HasPrefix(dataValue, "system:"):
		parts := strings.Split(dataValue, ":")
		if len(parts) != 3 || !validResourceType(parts[2]) {
			return nil, nil
		}
		data = domain.ConversationData{IntegrationID: parts[1], ResourceType: parts[2]}
		if _, err := s.activeIntegration(ctx, data.IntegrationID); err != nil {
			return nil, err
		}
		conversation.State = "SELECTING_RESOURCE"
		if data.ResourceType == "account" {
			return s.listResources(ctx, userID, chatID, callbackID, conversation, data, "")
		}
		return s.chooseAccount(ctx, userID, chatID, callbackID, conversation, data)
	case strings.HasPrefix(dataValue, "account:"):
		accountID := strings.TrimPrefix(dataValue, "account:")
		return s.listResources(ctx, userID, chatID, callbackID, conversation, data, accountID)
	case strings.HasPrefix(dataValue, "item:"):
		resourceID := strings.TrimPrefix(dataValue, "item:")
		return s.showResource(ctx, userID, chatID, callbackID, conversation, data, resourceID)
	case strings.HasPrefix(dataValue, "resource:"):
		return s.resourceAction(ctx, userID, chatID, callbackID, strings.TrimPrefix(dataValue, "resource:"), conversation, data)
	case dataValue == "confirm":
		return s.confirm(ctx, userID, strconv.FormatInt(chatID, 10), chatID, callbackID, "CONFIRMAR", conversation, data)
	case dataValue == "confirm-purge":
		return s.confirm(ctx, userID, strconv.FormatInt(chatID, 10), chatID, callbackID, "PURGAR", conversation, data)
	case dataValue == "menu:home":
		conversation.State = "MENU"
		return s.saveReply(ctx, conversation, data, menuReply(chatID, callbackID, "Menu Lynx OS"), userID)
	default:
		return nil, nil
	}
}

func (s *Service) collectProvisioning(ctx context.Context, userID, telegramID string, chatID int64, callbackID, text string, conversation domain.Conversation, data domain.ConversationData) (*domain.Reply, error) {
	integrations, err := s.integrationSystems(ctx)
	if err != nil {
		return nil, err
	}
	draft, err := s.extractor.Extract(ctx, text, data.Draft, integrations)
	if err != nil {
		return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Não consegui organizar essa mensagem. Continue em linguagem natural ou tente separar as informações em frases."}, nil
	}
	draft = mergeDraft(draft, data.Draft)
	normalizeDraft(&draft)
	data.Draft = draft
	selected := selectSystem(draft.IntegrationName, integrations)
	missing := missingFields(draft)
	if selected == nil {
		missing = append([]string{"sistema conectado (" + systemNames(integrations) + ")"}, missing...)
	}
	if len(missing) > 0 || selected == nil {
		conversation.State = "COLLECTING"
		return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Ainda preciso de: " + strings.Join(missing, ", ") + ". Pode mandar o que falta em uma ou mais frases."}, userID)
	}
	data.IntegrationID = selected.ID
	if data.IdempotencyKey == "" {
		key, err := newIdempotencyKey()
		if err != nil {
			return nil, err
		}
		data.IdempotencyKey = key
	}
	conversation.State = "AWAITING_CONFIRMATION"
	return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: formatPreview(draft, selected.Name), Buttons: [][]domain.Button{{{Text: "Confirmar cadastro", Data: "confirm"}, {Text: "Cancelar", Data: "menu:cancel"}}}}, userID)
}

func (s *Service) confirm(ctx context.Context, userID, telegramID string, chatID int64, callbackID, text string, conversation domain.Conversation, data domain.ConversationData) (*domain.Reply, error) {
	if data.PendingOperation != "" {
		if data.PendingOperation == "resource.purge" && !strings.EqualFold(text, "PURGAR") {
			return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Purga permanente exige que você digite PURGAR."}, nil
		}
		if data.PendingOperation != "resource.purge" && !strings.EqualFold(text, "CONFIRMAR") {
			return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Responda CONFIRMAR para executar ou CANCELAR para descartar."}, nil
		}
		if !s.userRoleAllows(ctx, userID, identitydomain.PermissionRunOperations) {
			return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Seu papel não permite executar alterações."}, nil
		}
		return s.executePending(ctx, userID, telegramID, chatID, callbackID, conversation, data)
	}
	if !strings.EqualFold(text, "CONFIRMAR") {
		return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Responda CONFIRMAR para executar ou CANCELAR para descartar."}, nil
	}
	if !s.userRoleAllows(ctx, userID, identitydomain.PermissionRunOperations) {
		return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Seu papel não permite executar alterações."}, nil
	}
	selected, err := s.activeIntegration(ctx, data.IntegrationID)
	if err != nil {
		return nil, err
	}
	payload := bootstrapCommand(data.Draft)
	requestID := "telegram-" + data.IdempotencyKey
	response, err := s.integrations.Execute(ctx, selected.ID, userID, requestID, payload, data.IdempotencyKey)
	if err != nil {
		return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "O SaaS não confirmou o cadastro. Responda CONFIRMAR para tentar de novo com a mesma chave segura, ou CANCELAR."}, userID)
	}
	conversation.State = "MENU"
	data = domain.ConversationData{}
	return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: formatOperationResult(response) + "\n\nEscolha a próxima ação:", Buttons: menuButtons()}, userID)
}

func (s *Service) chooseAccount(ctx context.Context, userID string, chatID int64, callbackID string, conversation domain.Conversation, data domain.ConversationData) (*domain.Reply, error) {
	response, err := s.integrations.Execute(ctx, data.IntegrationID, userID, "telegram-list-accounts", integrationdomain.Command{Operation: "resource.list", Input: map[string]any{"resourceType": "account", "filters": map[string]any{"lifecycle": "all"}, "page": map[string]any{"limit": 10}}}, "")
	if err != nil {
		return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Não consegui consultar as empresas agora."}, nil
	}
	items := resourceItems(response.Data)
	buttons := make([][]domain.Button, 0, len(items))
	for _, item := range items {
		buttons = append(buttons, []domain.Button{{Text: shortLabel(item), Data: "account:" + item.ID}})
	}
	if len(buttons) == 0 {
		return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Não há empresas disponíveis para essa consulta."}, nil
	}
	conversation.State = "SELECTING_RESOURCE"
	return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Escolha a empresa para listar " + resourceLabel(data.ResourceType) + ":", Buttons: buttons}, userID)
}

func (s *Service) listResources(ctx context.Context, userID string, chatID int64, callbackID string, conversation domain.Conversation, data domain.ConversationData, accountID string) (*domain.Reply, error) {
	input := map[string]any{"resourceType": data.ResourceType, "filters": map[string]any{"lifecycle": "all"}, "page": map[string]any{"limit": 10}}
	if accountID != "" {
		input["accountId"] = accountID
	}
	data.AccountID = accountID
	response, err := s.integrations.Execute(ctx, data.IntegrationID, userID, "telegram-list-"+data.ResourceType, integrationdomain.Command{Operation: "resource.list", Input: input}, "")
	if err != nil {
		return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Não consegui carregar a lista deste SaaS."}, nil
	}
	items := resourceItems(response.Data)
	buttons := make([][]domain.Button, 0, len(items))
	for _, item := range items {
		buttons = append(buttons, []domain.Button{{Text: shortLabel(item), Data: "item:" + item.ID}})
	}
	if len(buttons) == 0 {
		return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Nenhum registro encontrado."}, nil
	}
	conversation.State = "SELECTING_RESOURCE"
	return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Selecione " + resourceLabel(data.ResourceType) + ":", Buttons: buttons}, userID)
}

func (s *Service) showResource(ctx context.Context, userID string, chatID int64, callbackID string, conversation domain.Conversation, data domain.ConversationData, resourceID string) (*domain.Reply, error) {
	input := resourceInput(data.ResourceType, data.AccountID, resourceID)
	response, err := s.integrations.Execute(ctx, data.IntegrationID, userID, "telegram-get-"+resourceID, integrationdomain.Command{Operation: "resource.get", Input: input}, "")
	if err != nil {
		return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Não consegui abrir esse registro."}, nil
	}
	item := resourceObject(response.Data)
	if item == nil {
		return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "O SaaS não devolveu os detalhes do registro."}, nil
	}
	data.PendingInput = map[string]any{"resourceId": resourceID}
	conversation.State = "VIEWING_RESOURCE"
	capabilities, capErr := s.integrations.Capabilities(ctx, data.IntegrationID)
	buttons := [][]domain.Button{}
	if capErr == nil {
		ops := capabilities.Resources[data.ResourceType].Operations
		if includes(ops, "update") {
			buttons = append(buttons, []domain.Button{{Text: "Editar", Data: "resource:edit"}})
		}
		if includes(ops, "deactivate") && lifecycleState(item) != "hibernated" {
			buttons = append(buttons, []domain.Button{{Text: "Hibernar", Data: "resource:deactivate"}})
		}
		if includes(ops, "reactivate") && lifecycleState(item) == "hibernated" {
			buttons = append(buttons, []domain.Button{{Text: "Reativar", Data: "resource:reactivate"}})
		}
		if includes(ops, "purge") && lifecycleState(item) == "hibernated" {
			buttons = append(buttons, []domain.Button{{Text: "Purga definitiva", Data: "resource:purge"}})
		}
	}
	buttons = append(buttons, []domain.Button{{Text: "Voltar ao menu", Data: "menu:home"}})
	return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: formatResource(data.ResourceType, item), Buttons: buttons}, userID)
}

func (s *Service) resourceAction(ctx context.Context, userID string, chatID int64, callbackID, action string, conversation domain.Conversation, data domain.ConversationData) (*domain.Reply, error) {
	if action == "edit" {
		caps, err := s.integrations.Capabilities(ctx, data.IntegrationID)
		if err != nil {
			return nil, err
		}
		fields := editableFields(caps.Resources[data.ResourceType].Fields)
		if len(fields) == 0 {
			return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Este SaaS não permite editar campos deste recurso."}, nil
		}
		data.ResourceFields = fields
		conversation.State = "EDITING_RESOURCE"
		return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Envie os campos que deseja alterar, um por linha, no formato campo=valor. Campos: " + strings.Join(fields, ", ")}, userID)
	}
	if !s.userRoleAllows(ctx, userID, identitydomain.PermissionRunOperations) {
		return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Seu papel é somente de leitura."}, nil
	}
	operation := map[string]string{"deactivate": "resource.deactivate", "reactivate": "resource.reactivate", "purge": "resource.purge"}[action]
	if operation == "" {
		return nil, nil
	}
	resourceID, _ := data.PendingInput["resourceId"].(string)
	if resourceID == "" {
		return nil, nil
	}
	data.PendingOperation = operation
	data.PendingInput = resourceInput(data.ResourceType, data.AccountID, resourceID)
	key, err := newIdempotencyKey()
	if err != nil {
		return nil, err
	}
	data.IdempotencyKey = key
	conversation.State = "AWAITING_CONFIRMATION"
	confirmText := "Confirme a ação: " + operation + " sobre " + resourceLabel(data.ResourceType) + "."
	if operation == "resource.purge" {
		confirmText = "Esta purga é permanente e pode afetar registros relacionados. Digite PURGAR para continuar."
		return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: confirmText, Buttons: [][]domain.Button{{{Text: "Cancelar", Data: "menu:cancel"}}}}, userID)
	}
	return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: confirmText, Buttons: [][]domain.Button{{{Text: "Confirmar", Data: "confirm"}, {Text: "Cancelar", Data: "menu:cancel"}}}}, userID)
}

func (s *Service) collectResourceChanges(ctx context.Context, userID string, chatID int64, callbackID, text string, conversation domain.Conversation, data domain.ConversationData) (*domain.Reply, error) {
	changes := map[string]any{}
	allowed := map[string]bool{}
	for _, field := range data.ResourceFields {
		allowed[field] = true
	}
	for _, line := range strings.Split(text, "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Use uma linha por campo no formato campo=valor."}, nil
		}
		field := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if !allowed[field] || value == "" || len(value) > 240 {
			return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Campo não editável ou valor inválido. Campos permitidos: " + strings.Join(data.ResourceFields, ", ")}, nil
		}
		changes[field] = value
	}
	resourceID, _ := data.PendingInput["resourceId"].(string)
	input := resourceInput(data.ResourceType, data.AccountID, resourceID)
	input["changes"] = changes
	data.PendingInput = input
	data.PendingOperation = "resource.update"
	key, err := newIdempotencyKey()
	if err != nil {
		return nil, err
	}
	data.IdempotencyKey = key
	conversation.State = "AWAITING_CONFIRMATION"
	return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Vou alterar: " + formatChanges(changes) + ". Responda CONFIRMAR para enviar ou CANCELAR.", Buttons: [][]domain.Button{{{Text: "Confirmar", Data: "confirm"}, {Text: "Cancelar", Data: "menu:cancel"}}}}, userID)
}

func (s *Service) executePending(ctx context.Context, userID, telegramID string, chatID int64, callbackID string, conversation domain.Conversation, data domain.ConversationData) (*domain.Reply, error) {
	if data.IdempotencyKey == "" {
		key, err := newIdempotencyKey()
		if err != nil {
			return nil, err
		}
		data.IdempotencyKey = key
	}
	_, err := s.integrations.Execute(ctx, data.IntegrationID, userID, "telegram-"+data.IdempotencyKey, integrationdomain.Command{Operation: data.PendingOperation, Input: data.PendingInput}, data.IdempotencyKey)
	if err != nil {
		return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "O SaaS não confirmou a operação. Responda CONFIRMAR para tentar novamente com a mesma chave, ou CANCELAR."}, userID)
	}
	conversation.State = "MENU"
	data = domain.ConversationData{}
	return s.saveReply(ctx, conversation, data, &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: "Operação concluída no SaaS.\n\nEscolha a próxima ação:", Buttons: menuButtons()}, userID)
}

func (s *Service) saveReply(ctx context.Context, conversation domain.Conversation, data domain.ConversationData, reply *domain.Reply, userID string) (*domain.Reply, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	sealed, err := s.cipher.Encrypt(encoded)
	if err != nil {
		return nil, err
	}
	if userID != "" {
		conversation.AdminUserID = userID
	}
	if conversation.ExpiresAt.IsZero() || conversation.ExpiresAt.Before(s.now().Add(30*time.Minute)) {
		conversation.ExpiresAt = s.now().Add(24 * time.Hour)
	}
	if err := s.conversations.SaveConversation(ctx, conversation, sealed); err != nil {
		return nil, err
	}
	return reply, nil
}

func (s *Service) loadConversation(ctx context.Context, telegramID string) (domain.Conversation, domain.ConversationData, error) {
	conversation, sealed, err := s.conversations.GetConversation(ctx, telegramID)
	if errors.Is(err, domain.ErrConversationNotFound) {
		return domain.Conversation{TelegramUserID: telegramID, State: "MENU", ExpiresAt: s.now().Add(24 * time.Hour)}, domain.ConversationData{}, nil
	}
	if err != nil {
		return domain.Conversation{}, domain.ConversationData{}, err
	}
	if conversation.ExpiresAt.Before(s.now()) {
		conversation.State = "MENU"
		conversation.ExpiresAt = s.now().Add(24 * time.Hour)
		return conversation, domain.ConversationData{}, nil
	}
	plain, err := s.cipher.Decrypt(sealed)
	if err != nil {
		return domain.Conversation{}, domain.ConversationData{}, err
	}
	var data domain.ConversationData
	if err = json.Unmarshal(plain, &data); err != nil {
		return domain.Conversation{}, domain.ConversationData{}, err
	}
	return conversation, data, nil
}

func (s *Service) integrationSystems(ctx context.Context) ([]domain.System, error) {
	records, err := s.integrations.List(ctx)
	if err != nil {
		return nil, err
	}
	systems := []domain.System{}
	for _, record := range records {
		if record.Active && record.RevokedAt == nil {
			systems = append(systems, domain.System{ID: record.ID, Name: record.Name, Slug: record.Slug})
		}
	}
	return systems, nil
}
func (s *Service) activeIntegration(ctx context.Context, id string) (domain.System, error) {
	systems, err := s.integrationSystems(ctx)
	if err != nil {
		return domain.System{}, err
	}
	for _, system := range systems {
		if system.ID == id {
			return system, nil
		}
	}
	return domain.System{}, errors.New("integration is unavailable")
}

func mergeDraft(incoming, previous domain.Draft) domain.Draft {
	merge := func(a, b *string) *string {
		if a != nil && strings.TrimSpace(*a) != "" {
			v := strings.TrimSpace(*a)
			return &v
		}
		return b
	}
	return domain.Draft{IntegrationName: merge(incoming.IntegrationName, previous.IntegrationName), AccountName: merge(incoming.AccountName, previous.AccountName), AccountTaxID: merge(incoming.AccountTaxID, previous.AccountTaxID), AccountEmail: merge(incoming.AccountEmail, previous.AccountEmail), AccountAddress: merge(incoming.AccountAddress, previous.AccountAddress), CustomerTradeName: merge(incoming.CustomerTradeName, previous.CustomerTradeName), CustomerLegalName: merge(incoming.CustomerLegalName, previous.CustomerLegalName), CustomerTaxID: merge(incoming.CustomerTaxID, previous.CustomerTaxID), CustomerEmail: merge(incoming.CustomerEmail, previous.CustomerEmail), CustomerPhone: merge(incoming.CustomerPhone, previous.CustomerPhone), CustomerTaxRegime: merge(incoming.CustomerTaxRegime, previous.CustomerTaxRegime), RelationshipType: merge(incoming.RelationshipType, previous.RelationshipType), CustomerStatus: merge(incoming.CustomerStatus, previous.CustomerStatus), UserName: merge(incoming.UserName, previous.UserName), UserEmail: merge(incoming.UserEmail, previous.UserEmail), UserPhone: merge(incoming.UserPhone, previous.UserPhone)}
}

func normalizeDraft(draft *domain.Draft) {
	normalize := func(field **string, values ...string) {
		if *field == nil {
			return
		}
		value := strings.ToLower(strings.TrimSpace(**field))
		for _, valid := range values {
			if strings.HasPrefix(value, strings.ToLower(valid[:min(len(valid), 3)])) {
				v := valid
				*field = &v
				return
			}
		}
		*field = nil
	}
	normalize(&draft.RelationshipType, "Recorrente", "Avulso")
	normalize(&draft.CustomerStatus, "Ativo", "Inativo")
}

func missingFields(d domain.Draft) []string {
	fields := []struct {
		value *string
		label string
	}{{d.AccountName, "nome do escritório"}, {d.AccountTaxID, "CNPJ do escritório"}, {d.AccountEmail, "e-mail do escritório"}, {d.AccountAddress, "endereço do escritório"}, {d.CustomerTradeName, "nome fantasia do cliente"}, {d.CustomerTaxID, "CNPJ ou CPF do cliente"}, {d.CustomerTaxRegime, "regime tributário"}, {d.RelationshipType, "tipo de relação (Recorrente ou Avulso)"}, {d.CustomerStatus, "status do cliente (Ativo ou Inativo)"}, {d.UserName, "nome do usuário"}, {d.UserEmail, "e-mail de acesso"}}
	missing := []string{}
	for _, field := range fields {
		if field.value == nil || strings.TrimSpace(*field.value) == "" {
			missing = append(missing, field.label)
		}
	}
	return missing
}

func selectSystem(name *string, systems []domain.System) *domain.System {
	if name == nil {
		return nil
	}
	for i := range systems {
		if strings.EqualFold(strings.TrimSpace(*name), systems[i].Slug) || strings.EqualFold(strings.TrimSpace(*name), systems[i].Name) {
			return &systems[i]
		}
	}
	return nil
}
func systemNames(systems []domain.System) string {
	values := []string{}
	for _, item := range systems {
		values = append(values, item.Name)
	}
	if len(values) == 0 {
		return "nenhum sistema conectado"
	}
	return strings.Join(values, ", ")
}
func formatPreview(d domain.Draft, name string) string {
	return fmt.Sprintf("Vou cadastrar no %s:\n• Escritório: %s · %s\n  %s · %s\n• Cliente: %s · %s\n• Regime/tipo/status: %s · %s · %s\n• Usuário responsável: %s · %s\n\nConfirme no botão abaixo ou cancele.", name, value(d.AccountName), value(d.AccountTaxID), value(d.AccountEmail), value(d.AccountAddress), value(d.CustomerTradeName), value(d.CustomerTaxID), value(d.CustomerTaxRegime), value(d.RelationshipType), value(d.CustomerStatus), value(d.UserName), value(d.UserEmail))
}
func bootstrapCommand(d domain.Draft) integrationdomain.Command {
	return integrationdomain.Command{Operation: "bootstrap", Input: map[string]any{"account": map[string]any{"name": value(d.AccountName), "taxId": value(d.AccountTaxID), "email": value(d.AccountEmail), "address": value(d.AccountAddress)}, "initialCustomer": map[string]any{"tradeName": value(d.CustomerTradeName), "legalName": value(d.CustomerLegalName), "taxId": value(d.CustomerTaxID), "email": value(d.CustomerEmail), "phone": value(d.CustomerPhone), "taxRegime": value(d.CustomerTaxRegime), "relationshipType": value(d.RelationshipType), "status": value(d.CustomerStatus)}, "initialUser": map[string]any{"name": value(d.UserName), "email": value(d.UserEmail), "phone": value(d.UserPhone)}}}
}
func formatOperationResult(response integrationdomain.Response) string {
	var data map[string]any
	_ = json.Unmarshal(response.Data, &data)
	login, _ := data["login"].(map[string]any)
	url, _ := login["url"].(string)
	email, _ := login["email"].(string)
	if url != "" {
		return "Cadastro concluído no SaaS.\nLogin: " + url + "\nUsuário: " + email
	}
	return "Cadastro concluído no SaaS."
}
func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func newIdempotencyKey() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "tg-" + hex.EncodeToString(bytes), nil
}
func isStart(text string) bool {
	return text == "/start" || strings.HasPrefix(text, "/start@") || text == "menu:home"
}
func menuReply(chatID int64, callbackID, text string) *domain.Reply {
	return &domain.Reply{ChatID: chatID, CallbackQueryID: callbackID, Text: text, Buttons: menuButtons()}
}
func menuButtons() [][]domain.Button {
	return [][]domain.Button{{{Text: "Novo cadastro", Data: "menu:new"}}, {{Text: "Listar empresas", Data: "menu:list:account"}}, {{Text: "Listar clientes", Data: "menu:list:customer"}}, {{Text: "Listar usuários", Data: "menu:list:user"}}}
}
func validResourceType(value string) bool {
	return value == "account" || value == "customer" || value == "user"
}
func resourceLabel(value string) string {
	switch value {
	case "account":
		return "empresas"
	case "customer":
		return "clientes"
	default:
		return "usuários"
	}
}
func (s *Service) userRoleAllows(ctx context.Context, userID string, permission identitydomain.Permission) bool {
	user, err := s.identities.FindByID(ctx, userID)
	return err == nil && user.Active && user.Role.Allows(permission)
}

type resource struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	TradeName string         `json:"tradeName"`
	LegalName string         `json:"legalName"`
	Email     string         `json:"email"`
	TaxID     string         `json:"taxId"`
	Lifecycle map[string]any `json:"lifecycle"`
}

func resourceItems(raw json.RawMessage) []resource {
	var value struct {
		Items []resource `json:"items"`
	}
	_ = json.Unmarshal(raw, &value)
	return value.Items
}
func resourceObject(raw json.RawMessage) *resource {
	var item resource
	if err := json.Unmarshal(raw, &item); err != nil || item.ID == "" {
		return nil
	}
	return &item
}
func shortLabel(item resource) string {
	label := item.Name
	if label == "" {
		label = item.TradeName
	}
	if label == "" {
		label = item.LegalName
	}
	if label == "" {
		label = item.Email
	}
	if len(label) > 48 {
		label = label[:45] + "..."
	}
	return label + " · " + item.ID
}
func resourceInput(kind, accountID, id string) map[string]any {
	input := map[string]any{"resourceType": kind, "resourceId": id}
	if kind != "account" {
		input["accountId"] = accountID
	}
	return input
}
func lifecycleState(item *resource) string {
	if item == nil || item.Lifecycle == nil {
		return "active"
	}
	state, _ := item.Lifecycle["state"].(string)
	return state
}
func includes(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func editableFields(raw map[string]json.RawMessage) []string {
	var fields []string
	_ = json.Unmarshal(raw["editable"], &fields)
	return fields
}
func formatResource(kind string, item *resource) string {
	return fmt.Sprintf("%s\nID: %s\nNome: %s\nE-mail: %s\nDocumento: %s\nEstado: %s", resourceLabel(kind), item.ID, first(item.Name, item.TradeName, item.LegalName), item.Email, item.TaxID, lifecycleState(item))
}
func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "—"
}
func formatChanges(changes map[string]any) string {
	parts := []string{}
	for key, value := range changes {
		parts = append(parts, key+"="+fmt.Sprint(value))
	}
	return strings.Join(parts, ", ")
}

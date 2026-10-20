module Main exposing (main)

import Browser exposing (UrlRequest(..))
import Dict
import Browser.Navigation as Navigation exposing (Key)
import Html exposing (Html, button, div, h1, header, input, main_, nav, option, p, pre, section, select, span, table, tbody, td, text, tr)
import Html.Attributes exposing (class, colspan, href, placeholder, target, type_, value)
import Html.Events exposing (onClick, onInput, targetValue)
import Json.Decode as Decode
import Http
import Json.Decode as Decode
import Json.Encode as Encode
import Url exposing (Url)
import Url.Builder
import Url.Parser exposing ((</>), (<?>), parse, s, top)
import Url.Parser.Query as Query


main : Program () Model Msg
main =
    Browser.application
        { init = init
        , update = update
        , subscriptions = \_ -> Sub.none
        , view = view
        , onUrlChange = OnUrlChange
        , onUrlRequest = OnUrlRequest
        }



-- MODEL


type alias Model =
    { messages : List Message
    , total : Maybe Int
    , files : List MboxFile
    , query : String
    , folder : String
    , page : Int
    , detail : Detail
    , key : Navigation.Key
    , error : Maybe String
    }


type alias MboxFile =
    { id : Int
    , path : String
    , folder : String
    , size : Int
    , importedAt : String
    , messageCount : Int
    }


type alias Message =
    { id : Int
    , fromName : String
    , fromAddr : String
    , toAddrs : String
    , subject : String
    , date : String
    , snippet : String
    , ftsSnippet : Maybe String
    , mboxPath : String
    }


type Detail
    = Closed
    | Loading Int
    | Shown MessageDetail
    | RawLoading Int
    | RawShown Int String


type alias MessageDetail =
    { id : Int
    , fromName : String
    , fromAddr : String
    , toAddrs : String
    , subject : String
    , date : String
    , body : String
    , messageId : String
    , mboxPath : String
    , byteOffset : Int
    , byteSize : Int
    }


pageSize : Int
pageSize =
    50


init : () -> Url -> Key -> ( Model, Cmd Msg )
init _ url key =
    let
        r =
            Maybe.withDefault { q = "", page = 0, id = Nothing, folder = "" } (Url.Parser.parse routeParser url)
    in
    ( { messages = []
      , total = Nothing
      , files = []
      , query = r.q
      , folder = r.folder
      , page = r.page
      , detail = Closed
      , key = key
      , error = Nothing
      }
    , Cmd.batch [ fetchMessages r.q r.folder r.page, fetchStats, fetchFiles, maybeFetchDetail r.id ]
    )


type alias Route =
    { q : String, page : Int, id : Maybe Int, folder : String }


routeParser : Url.Parser.Parser (Route -> a) a
routeParser =
    Url.Parser.map
        (\q page id folder ->
            { q = Maybe.withDefault "" q, page = Maybe.withDefault 0 page, id = id, folder = Maybe.withDefault "" folder }
        )
        (Url.Parser.top
            <?> Query.string "q"
            <?> Query.int "page"
            <?> Query.int "id"
            <?> Query.string "folder"
        )


route : Model -> Maybe Int -> String
route model detailId =
    Url.Builder.absolute []
        (List.filterMap identity
            [ model.query |> nonEmpty |> Maybe.map (Url.Builder.string "q")
            , model.folder |> nonEmpty |> Maybe.map (Url.Builder.string "folder")
            , Just (Url.Builder.int "page" model.page)
            , detailId |> Maybe.map (Url.Builder.int "id")
            ]
        )


nonEmpty : String -> Maybe String
nonEmpty s =
    if s == "" then
        Nothing

    else
        Just s


maybeFetchDetail : Maybe Int -> Cmd Msg
maybeFetchDetail msgId =
    case msgId of
        Just id ->
            fetchDetail id

        Nothing ->
            Cmd.none



-- UPDATE


type Msg
    = OnUrlChange Url
    | OnUrlRequest Browser.UrlRequest
    | FetchedMessages (Result Http.Error (List Message))
    | FetchedStats (Result Http.Error Stats)
    | FetchedFiles (Result Http.Error (List MboxFile))
    | FetchedDetail Int (Result Http.Error MessageDetail)
    | FetchedRaw (Result Http.Error String)
    | SearchInput String
    | SearchKey Int
    | SubmitSearch
    | FolderChanged String
    | PrevPage
    | NextPage
    | OpenMessage Int
    | ShowRaw Int
    | ShowParsed
    | CloseDetail


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        OnUrlChange url ->
            init () url model.key
                |> Tuple.mapFirst (\m -> { m | key = model.key })

        OnUrlRequest (Browser.Internal url) ->
            ( model, Navigation.pushUrl model.key (Url.toString url) )

        OnUrlRequest (Browser.External href) ->
            ( model, Navigation.load href )

        FetchedMessages (Ok messages) ->
            ( { model | messages = messages, error = Nothing }, Cmd.none )

        FetchedMessages (Err err) ->
            ( { model | error = Just (httpError err) }, Cmd.none )

        FetchedStats (Ok stats) ->
            ( { model | total = Just stats.messages }, Cmd.none )

        FetchedStats (Err _) ->
            ( model, Cmd.none )

        FetchedFiles (Ok files) ->
            ( { model | files = files }, Cmd.none )

        FetchedFiles (Err _) ->
            ( model, Cmd.none )

        FetchedDetail _ (Ok detail) ->
            ( { model | detail = Shown detail }, Cmd.none )

        FetchedDetail id (Err err) ->
            ( { model | error = Just (httpError err), detail = Closed }, Cmd.none )

        FetchedRaw (Ok raw) ->
            ( { model | detail = RawShown 0 raw }, Cmd.none )

        FetchedRaw (Err err) ->
            ( { model | error = Just (httpError err) }, Cmd.none )

        SearchInput q ->
            ( { model | query = q }, Cmd.none )

        SearchKey code ->
            if code == 13 then
                ( { model | page = 0 }, Cmd.batch [ fetchMessages model.query model.folder 0, Navigation.pushUrl model.key (route { model | query = model.query, page = 0 } Nothing) ] )
            else
                ( model, Cmd.none )

        SubmitSearch ->
            ( { model | page = 0 }, Cmd.batch [ fetchMessages model.query model.folder 0, Navigation.pushUrl model.key (route { model | query = model.query, page = 0 } Nothing) ] )

        FolderChanged folder ->
            ( { model | folder = folder, page = 0, detail = Closed }
            , Cmd.batch
                [ fetchMessages model.query folder 0
                , Navigation.pushUrl model.key (route { model | folder = folder, page = 0 } Nothing)
                ]
            )

        PrevPage ->
            let
                page =
                    max 0 (model.page - 1)
            in
            ( { model | page = page, detail = Closed }, Cmd.batch [ fetchMessages model.query model.folder page, Navigation.pushUrl model.key (route { model | page = page } Nothing) ] )

        NextPage ->
            let
                page =
                    model.page + 1
            in
            ( { model | page = page, detail = Closed }, Cmd.batch [ fetchMessages model.query model.folder page, Navigation.pushUrl model.key (route { model | page = page } Nothing) ] )

        OpenMessage id ->
            ( { model | detail = Loading id }, Cmd.batch [ fetchDetail id, Navigation.pushUrl model.key (route model (Just id)) ] )

        ShowRaw id ->
            ( { model | detail = RawLoading id }, fetchRaw id )

        ShowParsed ->
            case model.detail of
                _ ->
                    -- re-fetch parsed view
                    case model.detail of
                        RawShown _ _ ->
                            ( model, model.messages |> List.head |> Maybe.map (\m -> fetchDetail m.id) |> Maybe.withDefault Cmd.none )

                        RawLoading id ->
                            ( model, fetchDetail id )

                        _ ->
                            ( model, Cmd.none )

        CloseDetail ->
            ( { model | detail = Closed }, Navigation.pushUrl model.key (route model Nothing) )


httpError : Http.Error -> String
httpError err =
    case err of
        Http.BadUrl url ->
            "bad url " ++ url

        Http.Timeout ->
            "timeout"

        Http.NetworkError ->
            "network error"

        Http.BadStatus code ->
            "server error " ++ String.fromInt code

        Http.BadBody body ->
            "unexpected response: " ++ body



-- API


fetchMessages : String -> String -> Int -> Cmd Msg
fetchMessages query folder page =
    Http.get
        { url =
            Url.Builder.absolute [ "api", "messages" ]
                (List.filterMap identity
                    [ query |> nonEmpty |> Maybe.map (Url.Builder.string "q")
                    , folder |> nonEmpty |> Maybe.map (Url.Builder.string "folder")
                    , Just (Url.Builder.int "limit" pageSize)
                    , Just (Url.Builder.int "offset" (page * pageSize))
                    ]
                )
        , expect = Http.expectJson FetchedMessages (Decode.list messageDecoder)
        }


fetchStats : Cmd Msg
fetchStats =
    Http.get
        { url = Url.Builder.absolute [ "api", "stats" ] []
        , expect = Http.expectJson FetchedStats statsDecoder
        }


fetchFiles : Cmd Msg
fetchFiles =
    Http.get
        { url = Url.Builder.absolute [ "api", "files" ] []
        , expect = Http.expectJson FetchedFiles (Decode.list mboxFileDecoder)
        }


fetchDetail : Int -> Cmd Msg
fetchDetail id =
    Http.get
        { url = Url.Builder.absolute [ "api", "messages", String.fromInt id ] []
        , expect = Http.expectJson (FetchedDetail id) messageDetailDecoder
        }


fetchRaw : Int -> Cmd Msg
fetchRaw id =
    Http.get
        { url =
            Url.Builder.absolute [ "api", "messages", String.fromInt id, "raw" ]
                [ Url.Builder.int "head" 65536 ]
        , expect = Http.expectString FetchedRaw
        }


messageDecoder : Decode.Decoder Message
messageDecoder =
    Decode.succeed Message
        |> required "id" Decode.int
        |> required "fromName" (nullableOr Decode.string "")
        |> required "fromAddr" (nullableOr Decode.string "")
        |> required "toAddrs" (nullableOr Decode.string "")
        |> required "subject" (nullableOr Decode.string "")
        |> required "date" (nullableOr Decode.string "")
        |> required "snippet" (nullableOr Decode.string "")
        |> required "ftsSnippet" (Decode.maybe Decode.string)
        |> required "mboxPath" (nullableOr Decode.string "")


defaultString : String -> Decode.Decoder String
defaultString field =
    Decode.field field (Decode.oneOf [ Decode.string, Decode.null "" ])


messageDetailDecoder : Decode.Decoder MessageDetail
messageDetailDecoder =
    Decode.succeed MessageDetail
        |> required "id" (nullableOr Decode.int 0)
        |> required "fromName" (nullableOr Decode.string "")
        |> required "fromAddr" (nullableOr Decode.string "")
        |> required "toAddrs" (nullableOr Decode.string "")
        |> required "subject" (nullableOr Decode.string "")
        |> required "date" (nullableOr Decode.string "")
        |> required "body" (nullableOr Decode.string "")
        |> required "messageId" (nullableOr Decode.string "")
        |> required "mboxPath" (nullableOr Decode.string "")
        |> required "byteOffset" (nullableOr Decode.int 0)
        |> required "byteSize" (nullableOr Decode.int 0)


required : String -> Decode.Decoder a -> Decode.Decoder (a -> b) -> Decode.Decoder b
required key valueDecoder rest =
    Decode.map2 (\value func -> func value) (Decode.field key valueDecoder) rest


nullableOr : Decode.Decoder a -> a -> Decode.Decoder a
nullableOr valueDecoder fallback =
    Decode.oneOf [ valueDecoder, Decode.null fallback, Decode.succeed fallback ]



statsDecoder : Decode.Decoder Stats
statsDecoder =
    Decode.map Stats (Decode.field "messages" Decode.int)


type alias Stats =
    { messages : Int }


mboxFileDecoder : Decode.Decoder MboxFile
mboxFileDecoder =
    Decode.succeed MboxFile
        |> required "id" Decode.int
        |> required "path" Decode.string
        |> required "folder" (nullableOr Decode.string "")
        |> required "size" Decode.int
        |> required "imported_at" Decode.string
        |> required "message_count" Decode.int



-- VIEW


view : Model -> Browser.Document Msg
view model =
    { title = "ez-forensics"
    , body =
        [ div [ class "app" ]
            [ header [ class "toolbar" ]
                [ h1 [] [ text "ez-forensics" ]
                , select
                    [ class "folder-select"
                    , value model.folder
                    , Html.Events.on "change"
                        (targetValue |> Decode.map FolderChanged)
                    ]
                    (folderOptions model.files)
                , input
                    [ type_ "search"
                    , placeholder "Search mail…"
                    , value model.query
                    , onInput SearchInput
                    , Html.Events.on "keydown"
                        (Decode.field "keyCode" Decode.int
                            |> Decode.map (\code -> if code == 13 then SearchKey code else SearchKey 0)
                        )
                    , Html.Attributes.autofocus True
                    ]
                    []
                , button [ onClick SubmitSearch ] [ text "Search" ]
                ]
            , main_ [ class "content" ]
                [ case model.error of
                    Just err ->
                        p [ class "error" ] [ text err ]

                    Nothing ->
                        text ""
                , viewList model
                ]
            , nav [ class "pager" ]
                [ button [ onClick PrevPage ] [ text "← Newer" ]
                , span [] [ text ("page " ++ String.fromInt (model.page + 1)) ]
                , button [ onClick NextPage ] [ text "Older →" ]
                ]
            ]
        , case model.detail of
                Closed ->
                    text ""

                _ ->
                    viewDetail model
        ]
    }


viewList : Model -> Html Msg
viewList model =
    table [ class "mail-list" ]
        [ tbody []
            (model.messages
                |> List.map
                    (\m ->
                        tr [ onClick (OpenMessage m.id), class "mail-row" ]
                            [ td [ class "date" ] [ text (shortDate m.date) ]
                            , td [ class "from" ] [ text (displayName m) ]
                            , td [ class "subject" ]
                                (div []
                                    [ span [ class "subject-line" ] [ text (orNoSubject m.subject) ]
                                    , span [ class "snippet" ] [ text (Maybe.withDefault m.snippet m.ftsSnippet) ]
                                    ]
                                    |> List.singleton
                                )
                            ]
                    )
            )
        ]


displayName : Message -> String
displayName m =
    displayNameOf m.fromName m.fromAddr


displayNameOf : String -> String -> String
displayNameOf name addr =
    if name /= "" then
        name

    else
        addr


orNoSubject : String -> String
orNoSubject s =
    if s == "" then
        "(no subject)"

    else
        s


shortDate : String -> String
shortDate d =
    case String.split "T" d of
        date :: _ ->
            date

        _ ->
            d


-- One option per source folder (group of mbox files), with the summed
-- message count. "All mail" (empty value) shows everything.
folderOptions : List MboxFile -> List (Html Msg)
folderOptions files =
    let
        total =
            List.sum (List.map .messageCount files)
    in
    [ option [ value "" ] [ text ("All mail (" ++ String.fromInt total ++ ")") ] ]
        ++ List.map
            (\( folder, folderFiles ) ->
                option
                    [ value folder ]
                    [ text (folderLabel folder ++ " (" ++ String.fromInt (List.sum (List.map .messageCount folderFiles)) ++ ")") ]
            )
            (files
                |> List.foldl
                    (\f acc ->
                        Dict.update f.folder
                            (\entries ->
                                Just (f :: Maybe.withDefault [] entries)
                            )
                            acc
                    )
                    Dict.empty
                |> Dict.toList
            )


folderLabel : String -> String
folderLabel folder =
    case List.reverse (String.split "/" folder) of
        last :: _ ->
            last

        [] ->
            folder


viewDetail : Model -> Html Msg
viewDetail model =
    case model.detail of
        Closed ->
            text ""

        Loading id ->
            section [ class "detail" ] [ text "loading…" ]

        RawLoading id ->
            section [ class "detail" ] [ text "loading raw…" ]

        RawShown _ raw ->
            section [ class "detail" ]
                [ header []
                    [ button [ onClick CloseDetail ] [ text "Close" ]
                    , button [ onClick ShowParsed ] [ text "Parsed" ]
                    ]
                , pre [] [ text raw ]
                ]

        Shown d ->
            section [ class "detail" ]
                [ header []
                    [ button [ onClick CloseDetail ] [ text "Close" ]
                    , button [ onClick (ShowRaw d.id) ] [ text "Raw" ]
                    ]
                , div [ class "detail-head" ]
                    [ h1 [] [ text (orNoSubject d.subject) ]
                    , p [] [ text (displayNameOf d.fromName d.fromAddr ++ " → " ++ d.toAddrs) ]
                    , p [ class "meta" ] [ text (d.date ++ "  ·  " ++ d.messageId) ]
                    , p [ class "meta" ] [ text ("source: " ++ d.mboxPath ++ " @" ++ String.fromInt d.byteOffset) ]
                    ]
                , div [ class "body" ] [ pre [] [ text d.body ] ]
                ]
